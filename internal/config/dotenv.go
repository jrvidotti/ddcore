package config

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// DotenvName is the per-environment file that sits next to ddcore.json.
const DotenvName = ".env"

// loadDotenv reads path and exports every variable it defines that is not
// already set in the real environment.
//
// The precedence is the whole point: a variable injected by the platform —
// Railway's DATABASE_URL, a `docker run -e` — must win over a file baked into
// the image, because the file is the developer's default and the environment
// is the deployment's decision. A missing file is not an error; most checkouts
// will not have one.
func loadDotenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	vars, err := parseDotenv(f)
	if err != nil {
		return err
	}
	for _, kv := range vars {
		if _, ok := os.LookupEnv(kv[0]); ok {
			continue
		}
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// parseDotenv reads KEY=value lines, returning them in file order.
//
// The grammar is deliberately the small one everybody already writes: blank
// lines and `#` comments are skipped, a leading `export ` is tolerated so a
// file can be `source`d by a shell too, and a value may be bare, 'single' or
// "double" quoted. Only inside double quotes does `\n` become a newline —
// bare and single-quoted values are taken literally, so a Postgres password
// full of backslashes survives.
//
// A line with no `=` is skipped rather than refused: a malformed line in a
// file this permissive is far more likely to be a stray note than a mistake
// worth refusing to boot over.
func parseDotenv(r io.Reader) ([][2]string, error) {
	var out [][2]string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out = append(out, [2]string{key, unquote(strings.TrimSpace(val))})
	}
	return out, sc.Err()
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return strings.ReplaceAll(v[1:len(v)-1], `\n`, "\n")
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	// An unquoted value ends at the first ` #`: `PORT=8090 # dev` is a port,
	// not a port with a comment glued to it.
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// env reads a variable, falling back to def when it is unset or empty.
func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envBool reads a boolean variable. Anything other than a recognised true
// value is false, and an unset variable keeps def.
func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
