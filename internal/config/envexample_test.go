package config

import (
	"os"
	"testing"
)

// The file people read is the repository's .env.example; the constant is
// what `ddcore init` writes. If the two drift apart, someone consulting the
// repository gets a list of variables that differs from what the server reads.
func TestEnvExampleMatchesTheCommittedFile(t *testing.T) {
	b, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("reading repository .env.example: %v", err)
	}
	if string(b) != EnvExample {
		t.Error(".env.example and config.EnvExample have drifted — update both")
	}
}
