package scaffold

import (
	"strings"
	"testing"
)

func TestComposeMatchesTheDSN(t *testing.T) {
	out, ok := Compose(LocalDSN("myapp", 5467))
	if !ok {
		t.Fatal("a localhost dsn should produce a compose file")
	}
	for _, want := range []string{
		`- "5467:5432"`,
		`POSTGRES_USER: "myapp"`,
		`POSTGRES_PASSWORD: "myapp"`,
		`POSTGRES_DB: "myapp"`,
		`"pg_isready -U myapp -d myapp"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "container_name") {
		t.Error("a fixed container_name makes two projects collide")
	}
}

func TestComposeQuotesAwkwardPasswords(t *testing.T) {
	out, ok := Compose("postgres://u:p%23a%3Ab%24c@127.0.0.1/d")
	if !ok {
		t.Fatal("127.0.0.1 is local")
	}
	if !strings.Contains(out, `POSTGRES_PASSWORD: "p#a:b$$c"`) {
		t.Fatalf("password not quoted and escaped:\n%s", out)
	}
	if !strings.Contains(out, `- "5432:5432"`) {
		t.Fatalf("default port missing:\n%s", out)
	}
}

func TestComposeSkipsRemoteDatabases(t *testing.T) {
	for _, dsn := range []string{
		"postgres://u:p@db.example.com:5432/d",
		"host=localhost user=u",
		"mysql://u:p@localhost/d",
	} {
		if _, ok := Compose(dsn); ok {
			t.Errorf("%s: expected no compose file", dsn)
		}
	}
}

func TestDBName(t *testing.T) {
	for in, want := range map[string]string{
		"my-library": "my_library",
		"MyApp":      "myapp",
		"2024-erp":   "_2024_erp",
		"---":        "ddcore",
		"ok_name":    "ok_name",
	} {
		if got := DBName(in); got != want {
			t.Errorf("DBName(%q) = %q, want %q", in, got, want)
		}
		if !DBNameRe.MatchString(DBName(in)) {
			t.Errorf("DBName(%q) = %q is not a valid --name", in, DBName(in))
		}
	}
}
