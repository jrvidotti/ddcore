package main

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// pgTool runs pg_dump or pg_restore. The connection string reaches the child
// without its password, which travels in PGPASSWORD instead: argv is readable
// by every user on the machine through `ps`, the environment is not.
type pgTool struct {
	name string // "pg_dump" or "pg_restore"
	path string
}

func findPGTool(name string) (*pgTool, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s was not found on PATH: install the PostgreSQL client tools (postgresql-client, at least as new as the server)", name)
	}
	return &pgTool{name: name, path: p}, nil
}

var pgVersionRe = regexp.MustCompile(`(\d+)(?:\.(\d+))?`)

// Version is the tool's major version and its full version string.
func (t *pgTool) Version(ctx context.Context) (int, string, error) {
	out, err := exec.CommandContext(ctx, t.path, "--version").Output()
	if err != nil {
		return 0, "", fmt.Errorf("%s --version: %w", t.name, err)
	}
	s := strings.TrimSpace(string(out))
	m := pgVersionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, s, fmt.Errorf("%s --version: cannot read %q", t.name, s)
	}
	major, _ := strconv.Atoi(m[1])
	return major, s, nil
}

// Run executes the tool with args plus the connection. Its stderr is returned
// in the error, redacted, because a failed dump is diagnosed from it.
func (t *pgTool) Run(ctx context.Context, dsn string, args []string, stdout *os.File) error {
	conn, password := splitDSNPassword(dsn)
	cmd := exec.CommandContext(ctx, t.path, append(args, "--dbname="+conn)...)
	cmd.Env = os.Environ()
	if password != "" {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+password)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2000 {
			msg = msg[:2000] + "…"
		}
		return fmt.Errorf("%s failed: %v: %s", t.name, err, redactText(msg, password))
	}
	return nil
}

var kvPassword = regexp.MustCompile(`(?i)\bpassword\s*=\s*('(?:[^'\\]|\\.|'')*'|\S+)`)

// splitDSNPassword removes the password from a URL or keyword/value
// connection string and returns it separately.
func splitDSNPassword(dsn string) (string, string) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil || u.User == nil {
			return dsn, ""
		}
		pw, ok := u.User.Password()
		if !ok {
			return dsn, ""
		}
		u.User = url.User(u.User.Username())
		return u.String(), pw
	}
	m := kvPassword.FindStringSubmatch(dsn)
	if m == nil {
		return dsn, ""
	}
	pw := m[1]
	if strings.HasPrefix(pw, "'") && strings.HasSuffix(pw, "'") && len(pw) >= 2 {
		pw = strings.ReplaceAll(strings.ReplaceAll(pw[1:len(pw)-1], `\'`, `'`), `''`, `'`)
	}
	return strings.TrimSpace(kvPassword.ReplaceAllString(dsn, "")), pw
}

func redactText(s, secret string) string {
	if secret != "" {
		s = strings.ReplaceAll(s, secret, "***")
	}
	return s
}
