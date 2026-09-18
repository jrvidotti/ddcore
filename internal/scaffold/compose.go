package scaffold

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// DBNameRe is what `ddcore init --name` accepts: the name becomes the user,
// the password and the database of the DSN, so it has to be a plain
// identifier that needs no quoting in a URL, in SQL or in YAML.
var DBNameRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// LocalDSN is the DSN `ddcore init --name n --db-port p` writes.
func LocalDSN(name string, port int) string {
	return fmt.Sprintf("postgres://%s:%s@localhost:%d/%s?sslmode=disable", name, name, port, name)
}

// DBName turns a directory name into a default for --name: lowercased, with
// anything outside [a-z0-9_] replaced, never starting with a digit.
func DBName(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(dir) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "ddcore"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	return s
}

// Compose returns a docker-compose.yml that runs the Postgres a DSN points
// at, with the same user, password, database and port. It reports false when
// the DSN is not a Postgres URL on this machine: a remote database is not
// something a local container can stand in for.
func Compose(dsn string) (string, bool) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", false
	}
	switch u.Hostname() {
	case "", "localhost", "127.0.0.1", "::1":
	default:
		return "", false
	}
	user := u.User.Username()
	if user == "" {
		user = "postgres"
	}
	pass, _ := u.User.Password()
	if pass == "" {
		pass = "postgres"
	}
	db := strings.TrimPrefix(u.Path, "/")
	if db == "" {
		db = user
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	// A JSON string is a valid YAML scalar, so quoting through json keeps a
	// password with ':' or '#' in it intact; '$$' stops Compose from reading a
	// '$' as a variable.
	q := func(s string) string {
		b, _ := json.Marshal(s)
		return strings.ReplaceAll(string(b), "$", "$$")
	}
	return fmt.Sprintf(`# Postgres for local development, matching the dsn in ddcore.json.
#   docker compose up -d     start it
#   docker compose down      stop it (docker compose down -v also deletes the data)
# Not for production: see https://ddcore.dev/guide/deployment
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    ports:
      - %s
    environment:
      POSTGRES_USER: %s
      POSTGRES_PASSWORD: %s
      POSTGRES_DB: %s
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", %s]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
`, q(port+":5432"), q(user), q(pass), q(db), q(fmt.Sprintf("pg_isready -U %s -d %s", shellQuote(user), shellQuote(db)))), true
}

func shellQuote(s string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
