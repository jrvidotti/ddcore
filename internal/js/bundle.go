package js

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/packages/sdk"
)

//go:embed desksdk.js
var deskSDKShim string

// App is an app directory to load.
type App struct {
	Name string
	Dir  string
	// Embedded is set for the built-in core app served from an fs.FS.
	Embedded fs.FS
}

// Bundle is the compiled server-side code of one app.
type Bundle struct {
	App   string
	Code  string
	Files []string // ts files included, relative to Dir
}

func sdkPlugin() api.Plugin {
	return api.Plugin{Name: "ddcore-sdk", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: `^@ddcore/sdk(/.*)?$`}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			p := "index.ts"
			if strings.HasPrefix(a.Path, "@ddcore/sdk/") {
				p = strings.TrimPrefix(a.Path, "@ddcore/sdk/") + ".ts"
			}
			return api.OnResolveResult{Path: p, Namespace: "ddcore-sdk"}, nil
		})
		b.OnResolve(api.OnResolveOptions{Filter: `^\./`, Namespace: "ddcore-sdk"}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			return api.OnResolveResult{Path: strings.TrimPrefix(a.Path, "./") + ".ts", Namespace: "ddcore-sdk"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "ddcore-sdk"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			data, err := sdk.FS.ReadFile("src/" + a.Path)
			if err != nil {
				return api.OnLoadResult{}, fmt.Errorf("@ddcore/sdk: %s does not exist", a.Path)
			}
			s := string(data)
			return api.OnLoadResult{Contents: &s, Loader: api.LoaderTS}, nil
		})
	}}
}

// embeddedPlugin serves an app from an fs.FS (the core app).
func embeddedPlugin(app App) api.Plugin {
	root := filepath.Clean(app.Dir)
	return api.Plugin{Name: "ddcore-embedded-" + app.Name, Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: `.*`}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			if strings.HasPrefix(a.Path, "@ddcore/") {
				return api.OnResolveResult{}, nil
			}
			var p string
			if strings.HasPrefix(a.Path, "/") || strings.HasPrefix(a.Path, root) {
				p = a.Path
			} else if a.Namespace == "ddcore-embedded" || a.Importer != "" {
				p = filepath.Join(filepath.Dir(a.Importer), a.Path)
			} else {
				p = filepath.Join(a.ResolveDir, a.Path)
			}
			p = strings.TrimPrefix(filepath.Clean(p), root+"/")
			if !strings.HasSuffix(p, ".ts") {
				p += ".ts"
			}
			return api.OnResolveResult{Path: filepath.Join(root, p), Namespace: "ddcore-embedded"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "ddcore-embedded"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			rel := strings.TrimPrefix(a.Path, root+"/")
			data, err := fs.ReadFile(app.Embedded, rel)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			s := string(data)
			return api.OnLoadResult{Contents: &s, Loader: api.LoaderTS, ResolveDir: filepath.Dir(a.Path)}, nil
		})
	}}
}

// ServerFiles lists the ts files that make up the server bundle of an app.
func ServerFiles(app App, includeTests bool) ([]string, error) {
	var files []string
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := path
		if app.Embedded == nil {
			rel, _ = filepath.Rel(app.Dir, path)
		}
		if d.IsDir() {
			base := d.Name()
			if rel != "." && (base == "node_modules" || base == "client" || base == ".ddcore" || strings.HasPrefix(base, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".d.ts") || strings.HasSuffix(rel, ".form.ts") {
			return nil
		}
		if strings.HasSuffix(rel, ".test.ts") && !includeTests {
			return nil
		}
		files = append(files, rel)
		return nil
	}
	var err error
	if app.Embedded != nil {
		err = fs.WalkDir(app.Embedded, ".", walk)
	} else {
		err = filepath.WalkDir(app.Dir, walk)
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return loadRank(files[i]) < loadRank(files[j]) || (loadRank(files[i]) == loadRank(files[j]) && files[i] < files[j])
	})
	return files, nil
}

// Doctypes load first so controllers/reports can rely on meta; tests last.
func loadRank(f string) int {
	switch {
	case f == "ddcore.app.ts":
		return 0
	case strings.HasSuffix(f, ".doctype.ts"):
		return 1
	case strings.HasSuffix(f, ".test.ts"):
		return 9
	}
	return 5
}

// appNameRe finds the `name` of the app manifest. It runs over the JS that
// esbuild produced, not over the TypeScript source, so comments and type
// annotations are already gone and cannot be mistaken for the declaration.
var appNameRe = regexp.MustCompile(`\bname\s*:\s*"([^"]+)"`)

// AppName is the namespace of the app in dir: the `name` declared in
// ddcore.app.ts, falling back to the directory's base name.
//
// The namespace is baked into every module path (see ModulePath), so it has to
// be known before the bundle exists — which is why it is read from the source
// instead of the manifest the runtime later evaluates. Deriving it from the
// directory instead is what made an app's identity depend on where it happened
// to be checked out: the same app answers to `demo` in apps/demo and to `app`
// under a Dockerfile's WORKDIR /app, and every method path moves with it.
//
// A manifest that cannot be read or that declares nothing usable keeps the old
// behaviour rather than failing the load; Engine.snapshot then compares this
// name against the one defineApp really registered and refuses a mismatch, so
// a wrong guess here surfaces as an error instead of a silent rename.
func AppName(dir string) string {
	fallback := filepath.Base(dir)
	src, err := os.ReadFile(filepath.Join(dir, "ddcore.app.ts"))
	if err != nil {
		return fallback
	}
	js, err := TransformTS(string(src))
	if err != nil {
		return fallback
	}
	m := appNameRe.FindStringSubmatch(js)
	if m == nil || !meta.ValidIdentAscii(m[1]) {
		return fallback
	}
	return m[1]
}

// ModulePath turns "services/api.ts" into "my_app.services.api".
func ModulePath(app, rel string) string {
	rel = strings.TrimSuffix(rel, ".ts")
	return app + "." + strings.ReplaceAll(rel, "/", ".")
}

// TransformTS strips TypeScript types from a standalone snippet, so `ddcore
// eval` and the MCP eval tool accept real TS instead of just JavaScript (B22).
func TransformTS(code string) (string, error) {
	res := api.Transform(code, api.TransformOptions{
		Loader: api.LoaderTS,
		Format: api.FormatDefault,
		Target: api.ES2020,
	})
	if len(res.Errors) > 0 {
		return "", fmt.Errorf("%s", res.Errors[0].Text)
	}
	return string(res.Code), nil
}

// BuildServer bundles one app into a CommonJS script that registers every
// module in the runtime registry.
func BuildServer(app App, includeTests bool) (*Bundle, error) {
	files, err := ServerFiles(app, includeTests)
	if err != nil {
		return nil, err
	}
	var entry strings.Builder
	entry.WriteString("__ddcore.app = " + fmt.Sprintf("%q", app.Name) + ";\n")
	for _, f := range files {
		mp := ModulePath(app.Name, f)
		fmt.Fprintf(&entry, "__ddcore.current = %q; __ddcore.register('module', { path: %q, file: %q, exports: require(%q) });\n", mp, mp, f, "./"+f)
	}
	plugins := []api.Plugin{sdkPlugin()}
	if app.Embedded != nil {
		plugins = append(plugins, embeddedPlugin(app))
	}
	res := api.Build(api.BuildOptions{
		Stdin:         &api.StdinOptions{Contents: entry.String(), ResolveDir: app.Dir, Sourcefile: "__entry.ts", Loader: api.LoaderTS},
		Bundle:        true,
		Write:         false,
		Format:        api.FormatCommonJS,
		Platform:      api.PlatformNeutral,
		Target:        api.ES2020,
		Plugins:       plugins,
		LogLevel:      api.LogLevelSilent,
		Sourcemap:     api.SourceMapInline,
		Define:        map[string]string{"process.env.NODE_ENV": `"production"`},
		AbsWorkingDir: absDir(app.Dir),
	})
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("erro ao compilar app %s:\n%s", app.Name, formatMessages(res.Errors))
	}
	return &Bundle{App: app.Name, Code: string(res.OutputFiles[0].Contents), Files: files}, nil
}

// BuildServerBundle compiles an app's server-side bundle.
func BuildServerBundle(app App, includeTests bool) (*Bundle, error) {
	return BuildServer(app, includeTests)
}

func absDir(d string) string {
	if a, err := filepath.Abs(d); err == nil {
		return a
	}
	return d
}

func formatMessages(msgs []api.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Location != nil {
			fmt.Fprintf(&b, "  %s:%d:%d: %s\n", m.Location.File, m.Location.Line, m.Location.Column, m.Text)
		} else {
			fmt.Fprintf(&b, "  %s\n", m.Text)
		}
	}
	return b.String()
}

// BuildClient bundles a browser entry (form script or desk include) to ESM.
// `@ddcore/desk-sdk` resolves to the runtime the desk exposes on window.
func BuildClient(app App, entry string) (string, error) {
	shim := deskSDKShim
	plugin := api.Plugin{Name: "ddcore-desk-sdk", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: `^@ddcore/desk-sdk$`}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			return api.OnResolveResult{Path: "desk-sdk", Namespace: "ddcore-desk"}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "ddcore-desk"}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			return api.OnLoadResult{Contents: &shim, Loader: api.LoaderJS}, nil
		})
	}}
	plugins := []api.Plugin{plugin}
	if app.Embedded != nil {
		plugins = append(plugins, embeddedPlugin(app))
	}
	res := api.Build(api.BuildOptions{
		EntryPoints:   []string{filepath.Join(app.Dir, entry)},
		Bundle:        true,
		Write:         false,
		Format:        api.FormatESModule,
		Platform:      api.PlatformBrowser,
		Target:        api.ES2020,
		Plugins:       plugins,
		LogLevel:      api.LogLevelSilent,
		Sourcemap:     api.SourceMapInline,
		AbsWorkingDir: absDir(app.Dir),
	})
	if len(res.Errors) > 0 {
		return "", fmt.Errorf("erro ao compilar %s/%s:\n%s", app.Name, entry, formatMessages(res.Errors))
	}
	return string(res.OutputFiles[0].Contents), nil
}

// ListFiles returns files under dir matching the suffix, relative to dir.
func ListFiles(app App, suffix string) []string {
	var out []string
	if app.Embedded != nil {
		fs.WalkDir(app.Embedded, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, suffix) {
				out = append(out, p)
			}
			return nil
		})
		return out
	}
	filepath.WalkDir(app.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && p != app.Dir {
			return fs.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, suffix) {
			rel, _ := filepath.Rel(app.Dir, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// ReadFile reads a file from the app (disk or embedded).
func ReadFile(app App, rel string) ([]byte, error) {
	if app.Embedded != nil {
		return fs.ReadFile(app.Embedded, rel)
	}
	return os.ReadFile(filepath.Join(app.Dir, rel))
}
