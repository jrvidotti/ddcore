package engine

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// External databases: a read-only door from server code to a database that is
// not the site's own — a reporting copy of an ERP, say.
//
// An app cannot compile a driver into the binary, so the framework carries
// one, and it keeps the same rules as the rest of the integration surface:
// the credentials come from the environment (`DDCORE_SECRET_<NAME>_*`), never
// from a document, so they reach no backup; and the connection only reads,
// like `ddcore.db.sql`.

const (
	extDefaultPort    = "1433"
	extDefaultTimeout = 30 * time.Second
	extMaxOpenConns   = 4
)

// externalDBConfig is one external database as the environment describes it.
type externalDBConfig struct {
	Name     string
	Driver   string
	Host     string
	Port     string
	Database string
	User     string
	Password string
	Encrypt  string
}

// externalDBConfig reads DDCORE_SECRET_<NAME>_HOST, _PORT, _DATABASE, _USER,
// _PASSWORD, and the optional _DRIVER and _ENCRYPT. A missing variable is
// named in the error; its value never is.
func (e *Engine) externalDBConfig(name string) (externalDBConfig, error) {
	if strings.TrimSpace(name) == "" {
		return externalDBConfig{}, cerr.Validation("ddcore.externalDb: provide a name")
	}
	prefix := SecretEnvName(name)
	get := func(suffix string) (string, string) {
		v, _ := os.LookupEnv(prefix + suffix)
		return strings.TrimSpace(v), prefix + suffix
	}
	cfg := externalDBConfig{Name: name}
	for _, req := range []struct {
		suffix string
		dst    *string
	}{
		{"_HOST", &cfg.Host},
		{"_DATABASE", &cfg.Database},
		{"_USER", &cfg.User},
		{"_PASSWORD", &cfg.Password},
	} {
		v, env := get(req.suffix)
		if v == "" {
			return externalDBConfig{}, cerr.Validation("External database {0}: {1} is not configured", name, env)
		}
		*req.dst = v
	}
	cfg.Port, _ = get("_PORT")
	if cfg.Port == "" {
		cfg.Port = extDefaultPort
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		_, env := get("_PORT")
		return externalDBConfig{}, cerr.Validation("External database {0}: {1} is not a port number", name, env)
	}
	cfg.Driver, _ = get("_DRIVER")
	cfg.Driver = strings.ToLower(cfg.Driver)
	if cfg.Driver == "" {
		cfg.Driver = "sqlserver"
	}
	if cfg.Driver != "sqlserver" {
		_, env := get("_DRIVER")
		return externalDBConfig{}, cerr.Validation("External database {0}: {1} names an unsupported driver (only sqlserver)", name, env)
	}
	cfg.Encrypt, _ = get("_ENCRYPT")
	return cfg, nil
}

// dsn is the go-mssqldb connection URL. ApplicationIntent=ReadOnly routes to
// a readable secondary where there is one; the real boundary is a read-only
// login, which the docs recommend.
func (cfg externalDBConfig) dsn() string {
	q := url.Values{}
	q.Set("database", cfg.Database)
	q.Set("app name", "ddcore")
	q.Set("ApplicationIntent", "ReadOnly")
	if cfg.Encrypt != "" {
		q.Set("encrypt", cfg.Encrypt)
	}
	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     net.JoinHostPort(cfg.Host, cfg.Port),
		RawQuery: q.Encode(),
	}
	return u.String()
}

// redact keeps the password out of an error a driver might build from the DSN.
func (cfg externalDBConfig) redact(err error) string {
	s := err.Error()
	if cfg.Password != "" {
		s = strings.ReplaceAll(s, cfg.Password, "***")
		s = strings.ReplaceAll(s, url.QueryEscape(cfg.Password), "***")
	}
	return s
}

type extPool struct {
	key string
	db  *sql.DB
}

// externalPool opens one small pool per name and keeps it, in the pattern of
// oidcClientFor. A changed configuration builds a new pool and closes the old.
func (e *Engine) externalPool(name string) (*sql.DB, externalDBConfig, error) {
	cfg, err := e.externalDBConfig(name)
	if err != nil {
		return nil, cfg, err
	}
	key := cfg.dsn()
	e.extDBMu.Lock()
	defer e.extDBMu.Unlock()
	if p := e.extDB[name]; p != nil {
		if p.key == key {
			return p.db, cfg, nil
		}
		p.db.Close()
		delete(e.extDB, name)
	}
	conn, err := sql.Open("sqlserver", key)
	if err != nil {
		return nil, cfg, cerr.Validation("External database {0}: {1}", name, cfg.redact(err))
	}
	// A report must not be able to flood the other side with connections.
	conn.SetMaxOpenConns(extMaxOpenConns)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxIdleTime(5 * time.Minute)
	if e.extDB == nil {
		e.extDB = map[string]*extPool{}
	}
	e.extDB[name] = &extPool{key: key, db: conn}
	return conn, cfg, nil
}

// isReadQuery is the first filter both db.sql and externalDb().sql apply. It
// is not the boundary — a transaction that is never committed is.
func isReadQuery(query string) bool {
	q := strings.TrimSpace(strings.ToLower(query))
	return strings.HasPrefix(q, "select") || strings.HasPrefix(q, "with")
}

// ExternalSQL runs a read-only query on a named external database. The query
// runs inside a transaction that is always rolled back, with positional
// parameters (@p1, @p2, …) that are never interpolated.
func (c *Ctx) ExternalSQL(name, query string, params []any, timeout float64) ([]map[string]any, error) {
	if !isReadQuery(query) {
		return nil, cerr.Permission("ddcore.externalDb().sql only accepts SELECT")
	}
	conn, cfg, err := c.E.externalPool(name)
	if err != nil {
		return nil, err
	}
	d := extDefaultTimeout
	if timeout > 0 {
		d = time.Duration(timeout * float64(time.Second))
	}
	parent := c.Ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()
	fail := func(err error) error {
		return cerr.Validation("External database {0}: {1}", name, cfg.redact(err))
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fail(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fail(err)
	}
	defer rows.Close()
	out, err := scanExternalRows(rows)
	if err != nil {
		return nil, fail(err)
	}
	return out, nil
}

func scanExternalRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, ct := range cols {
			row[ct.Name()] = normalizeMSSQL(vals[i], ct.DatabaseTypeName())
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// maxSafeInt is the largest integer a JS number holds exactly.
const maxSafeInt = 1<<53 - 1

// normalizeMSSQL converts a go-mssqldb value the way db.Normalize converts a
// pgx one, using the column's type name where the Go value alone is ambiguous.
func normalizeMSSQL(v any, typ string) any {
	if v == nil {
		return nil
	}
	switch strings.ToUpper(typ) {
	case "DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY":
		var s string
		switch x := v.(type) {
		case []byte:
			s = string(x)
		case string:
			s = x
		default:
			return db.Normalize(v)
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f
		}
		return s
	case "DATE":
		if t, ok := v.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	case "TIME":
		if t, ok := v.(time.Time); ok {
			return t.Format("15:04:05")
		}
	case "DATETIME", "DATETIME2", "SMALLDATETIME", "DATETIMEOFFSET":
		if t, ok := v.(time.Time); ok {
			return t.Format(time.RFC3339Nano)
		}
	case "UNIQUEIDENTIFIER":
		if b, ok := v.([]byte); ok {
			var u mssql.UniqueIdentifier
			if err := u.Scan(b); err == nil {
				return u.String()
			}
		}
	case "BIGINT":
		if n, ok := v.(int64); ok && (n > maxSafeInt || n < -maxSafeInt) {
			return strconv.FormatInt(n, 10)
		}
	}
	return db.Normalize(v)
}

// ExternalDBInfo is what `ddcore doctor` shows of an external database: where
// it points, never the credentials.
type ExternalDBInfo struct {
	Name     string `json:"name"`
	Host     string `json:"host,omitempty"`
	Database string `json:"database,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ExternalDBs lists the external databases the environment configures: every
// secret prefix that has both a _HOST and a _DATABASE.
func (e *Engine) ExternalDBs() []ExternalDBInfo {
	have := map[string]bool{}
	for _, n := range e.SecretNames() {
		have[n] = true
	}
	var out []ExternalDBInfo
	for n := range have {
		base, ok := strings.CutSuffix(n, "_HOST")
		if !ok || base == "" || !have[base+"_DATABASE"] {
			continue
		}
		name := strings.ToLower(base)
		info := ExternalDBInfo{Name: name}
		if cfg, err := e.externalDBConfig(name); err != nil {
			info.Error = err.Error()
		} else {
			info.Host = net.JoinHostPort(cfg.Host, cfg.Port)
			info.Database = cfg.Database
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ExternalDBPing checks that an external database answers, for `ddcore doctor`.
func (e *Engine) ExternalDBPing(ctx context.Context, name string) error {
	conn, cfg, err := e.externalPool(name)
	if err != nil {
		return err
	}
	var one int
	if err := conn.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return cerr.Validation("External database {0}: {1}", name, cfg.redact(err))
	}
	return nil
}
