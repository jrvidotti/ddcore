package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCommandTableNamesAreUniqueAndRunnable(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands() {
		if c.name == "" || c.run == nil {
			t.Fatalf("command %+v needs a name and a run", c)
		}
		if seen[c.name] {
			t.Fatalf("command %q is listed twice", c.name)
		}
		seen[c.name] = true
	}
}

func TestUsageTextListsEveryDocumentedCommandInOrder(t *testing.T) {
	text := usageText()
	at := 0
	for _, c := range commands() {
		if c.summary == "" {
			if strings.Contains(text, "  "+c.name+" ") {
				t.Fatalf("hidden command %q is in the usage text", c.name)
			}
			continue
		}
		line := fmt.Sprintf("  %-11s %s\n", c.name, c.summary)
		i := strings.Index(text[at:], line)
		if i < 0 {
			t.Fatalf("usage text lacks %q after offset %d:\n%s", line, at, text)
		}
		at += i + len(line)
	}
	if !strings.HasPrefix(text, "ddcore — ") || !strings.HasSuffix(text, "`ddcore docs backup`.\n") {
		t.Fatalf("usage text lost its head or tail:\n%s", text)
	}
}

func TestEverySubcommandUsageIsSet(t *testing.T) {
	for _, name := range []string{"import", "jobs", "audit", "webhooks", "push", "maintenance", "tenant", "deploy", "user", "i18n"} {
		c, ok := findCommand(name)
		if !ok || strings.TrimSpace(c.usage) == "" {
			t.Fatalf("%s has no usage text", name)
		}
	}
}

func TestDispatchRunsTheNamedSubcommandWithTheRest(t *testing.T) {
	var got []string
	subs := []subcommand{
		{"a", func(args []string) error { got = append([]string{"a"}, args...); return nil }},
		{"b", func(args []string) error { return errors.New("b failed") }},
	}
	if err := dispatch("usage: x\n", subs, []string{"a", "1", "2"}); err != nil || strings.Join(got, " ") != "a 1 2" {
		t.Fatalf("a: err=%v got=%v", err, got)
	}
	if err := dispatch("usage: x\n", subs, []string{"b"}); err == nil || err.Error() != "b failed" {
		t.Fatalf("b: err=%v", err)
	}
}

func TestDispatchWithoutASubcommandReturnsTheUsage(t *testing.T) {
	err := dispatch("usage: x", nil, nil)
	if err == nil || err.Error() != "usage: x" {
		t.Fatalf("err = %v", err)
	}
}

func TestDispatchNamesAnUnknownSubcommandAndShowsTheUsage(t *testing.T) {
	err := dispatch("usage: x\n", []subcommand{{"a", nil}}, []string{"zzz"})
	if err == nil || err.Error() != "unknown subcommand: zzz\n\nusage: x\n" {
		t.Fatalf("err = %v", err)
	}
}

func TestExitStatus(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		msg    string
	}{
		{nil, 0, ""},
		{errHelpShown, 0, ""},
		{fmt.Errorf("wrapped: %w", errHelpShown), 0, ""},
		{exitCode(2), 2, ""},
		{fmt.Errorf("wrapped: %w", exitCode(3)), 3, ""},
		{errors.New("boom"), 1, "error: boom"},
	} {
		if status, msg := exitStatus(c.err); status != c.status || msg != c.msg {
			t.Errorf("exitStatus(%v) = %d, %q; want %d, %q", c.err, status, msg, c.status, c.msg)
		}
	}
}

func TestRunWithNoCommandExitsTwo(t *testing.T) {
	defer discardStdout(t)()
	if err := run(nil); err != exitCode(2) {
		t.Fatalf("err = %v, want exit 2", err)
	}
}

// discardStdout silences what a command prints, and returns the restore.
func discardStdout(t *testing.T) func() {
	t.Helper()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = null
	return func() { os.Stdout = old; null.Close() }
}
