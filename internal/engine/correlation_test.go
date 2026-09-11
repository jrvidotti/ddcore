package engine

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestLogHandlerWritesWhereTheConfigSays(t *testing.T) {
	var buf bytes.Buffer
	slog.New(logHandler(Config{LogOut: &buf, LogLevel: slog.LevelInfo})).Info("request", "id", "abc")
	if got := buf.String(); !strings.Contains(got, "msg=request") || !strings.Contains(got, "id=abc") {
		t.Fatalf("the line did not reach the configured writer: %q", got)
	}
	buf.Reset()
	slog.New(logHandler(Config{LogOut: &buf, LogLevel: slog.LevelInfo, LogJSON: true})).Info("request")
	// A platform reads the level out of this field; without it every line is
	// whatever the stream it arrived on suggests.
	if got := buf.String(); !strings.Contains(got, `"level":"INFO"`) || !strings.Contains(got, `"msg":"request"`) {
		t.Fatalf("JSON line lacks the fields a collector reads: %q", got)
	}
}

// stderr means "this is an error" to anything that captures both streams, so
// the destination nobody configured has to be the other one.
func TestLogHandlerDefaultsToStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	saved := os.Stdout
	os.Stdout = w
	slog.New(logHandler(Config{LogLevel: slog.LevelInfo})).Info("request")
	os.Stdout = saved
	w.Close()

	var got bytes.Buffer
	if _, err := got.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.String(), "msg=request") {
		t.Fatalf("nothing reached stdout: %q", got.String())
	}
}
