package i18nx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Target is one catalogue and everything that feeds it. The framework's own
// catalogue (`core`) is fed by the desk, the Go core and the core DocTypes; an
// app's is fed by its own directory and its own metadata. A key always belongs
// to exactly one target, so `--check` per target is meaningful.
type Target struct {
	App    string   // app name, as the registry knows it
	Dir    string   // where translations/<lang>.csv lives
	Root   string   // repo root, for the paths written into the CSV
	TSDirs []string // TypeScript/Svelte roots
	GoDirs []string // Go roots
}

func (t Target) CatalogPath(lang string) string {
	return filepath.Join(t.Dir, "translations", lang+".csv")
}

// Targets builds the extraction targets for a checkout. `core` is included
// only when the framework source is on disk: an app developer using a
// released binary has no core/ to extract from.
func Targets(e *engine.Engine, root string, only string) []Target {
	var out []Target
	st := e.Current()
	for _, a := range st.Apps {
		dir := a.Dir
		if a.Name == "core" {
			dir = filepath.Join(root, "core")
			if _, err := os.Stat(dir); err != nil {
				continue
			}
		}
		if dir == "" {
			continue
		}
		if only != "" && only != a.Name {
			continue
		}
		t := Target{App: a.Name, Dir: dir, Root: root, TSDirs: []string{dir}}
		if a.Name == "core" {
			// the desk is the framework's interface, and the Go core raises
			// the errors a user reads: both belong to the core catalogue.
			t.TSDirs = append(t.TSDirs, filepath.Join(root, "desk", "src"), filepath.Join(root, "packages"))
			t.GoDirs = []string{filepath.Join(root, "internal"), filepath.Join(root, "cmd")}
		}
		out = append(out, t)
	}
	return out
}

// Extract runs the three collectors for one target.
func Extract(e *engine.Engine, t Target) (*Set, error) {
	s := NewSet()
	for _, dir := range t.TSDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := CollectTS(s, dir, t.Root, skipTS); err != nil {
			return nil, err
		}
	}
	for _, dir := range t.GoDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := CollectGo(s, dir, t.Root); err != nil {
			return nil, err
		}
	}
	collectMeta(s, e, t)
	return s, nil
}

// skipTS drops what is not interface text. Tests are the whole of it: the
// generated `.ddcore/` mirrors are already skipped as dot-directories.
func skipTS(path string) bool {
	return strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.svelte")
}

func collectMeta(s *Set, e *engine.Engine, t Target) {
	st := e.Current()
	// every DocType, not just this app's: an extension's fields live on
	// someone else's DocType and their text is still this app's to translate
	for _, name := range st.Meta.Names() {
		d := st.Meta.DocTypes[name]
		if d.App != t.App && !containsApp(d.ExtendedBy, t.App) {
			continue
		}
		CollectDocType(s, d, t.App, metaRef(t, d))
	}
	for _, ws := range st.Snap.Workspaces {
		if appOf(ws) == t.App {
			CollectTree(s, map[string]any(ws), t.App+" workspace")
		}
	}
	for _, rep := range st.Snap.Reports {
		if appOf(rep) == t.App {
			CollectReport(s, map[string]any(rep), t.App+" report")
		}
	}
	for _, wf := range st.Snap.Workflows {
		if wf.App == t.App {
			CollectWorkflow(s, wf, workflowRef(t, wf))
		}
	}
	for _, pt := range st.Snap.PrintTemplates {
		if pt.App == t.App {
			CollectPrintTemplate(s, pt, printTemplateRef(t, pt))
		}
	}
	if am, ok := st.Snap.Apps[t.App]; ok {
		var tree map[string]any
		if b, err := json.Marshal(am); err == nil {
			json.Unmarshal(b, &tree)
			// an app's `roles` are identifiers, and `title`/`description` are
			// the only text defineApp carries.
			delete(tree, "fixtures")
			CollectTree(s, tree, t.App+" app")
		}
	}
}

func containsApp(list []string, app string) bool {
	for _, x := range list {
		if x == app {
			return true
		}
	}
	return false
}

func appOf(m map[string]any) string {
	s, _ := m["app"].(string)
	return s
}

// metaRef points the CSV comment at whatever identifies the DocType's source.
// SourceFile is a module id ("core.doctypes.role.role.doctype"), not a path,
// so it is used verbatim unless it happens to be one.
func metaRef(t Target, d *meta.DocType) string {
	if d.SourceFile == "" {
		return t.App + " meta"
	}
	if !strings.ContainsRune(d.SourceFile, '/') {
		return d.SourceFile
	}
	if rel, err := filepath.Rel(t.Root, filepath.Join(t.Dir, d.SourceFile)); err == nil {
		return rel
	}
	return d.SourceFile
}

// printTemplateRef names the print template's source module, or the app when
// unknown.
func printTemplateRef(t Target, pt engine.PrintTemplate) string {
	if pt.SourceFile == "" {
		return t.App + " print"
	}
	return pt.SourceFile
}

// workflowRef names the workflow's source module, or the app when unknown.
func workflowRef(t Target, wf js.Workflow) string {
	if wf.SourceFile == "" {
		return t.App + " workflow"
	}
	return wf.SourceFile
}
