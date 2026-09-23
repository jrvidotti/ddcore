package engine

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

func setExternalEnv(t *testing.T, prefix string, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(prefix+k, v)
	}
}

func TestExternalDBConfig(t *testing.T) {
	e := &Engine{}
	setExternalEnv(t, "DDCORE_SECRET_SQL_SERVER", map[string]string{
		"_HOST": "erp.example.com", "_DATABASE": "ERP", "_USER": "reader",
		"_PASSWORD": "p@ss:w/rd?&x", "_ENCRYPT": "true",
	})
	cfg, err := e.externalDBConfig("sql_server")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "1433" || cfg.Driver != "sqlserver" {
		t.Fatalf("defaults: %+v", cfg)
	}
	u, err := url.Parse(cfg.dsn())
	if err != nil {
		t.Fatal(err)
	}
	if pw, _ := u.User.Password(); pw != "p@ss:w/rd?&x" || u.User.Username() != "reader" {
		t.Fatalf("credentials did not round-trip: %q", cfg.dsn())
	}
	q := u.Query()
	if u.Host != "erp.example.com:1433" || q.Get("database") != "ERP" ||
		q.Get("ApplicationIntent") != "ReadOnly" || q.Get("encrypt") != "true" {
		t.Fatalf("dsn: %s", cfg.dsn())
	}

	t.Setenv("DDCORE_SECRET_SQL_SERVER_PORT", "14330")
	if cfg, _ := e.externalDBConfig("sql_server"); cfg.Port != "14330" {
		t.Fatalf("port: %q", cfg.Port)
	}
	t.Setenv("DDCORE_SECRET_SQL_SERVER_PORT", "nope")
	if _, err := e.externalDBConfig("sql_server"); err == nil {
		t.Fatal("a non-numeric port was accepted")
	}
	t.Setenv("DDCORE_SECRET_SQL_SERVER_PORT", "")

	t.Setenv("DDCORE_SECRET_SQL_SERVER_DRIVER", "oracle")
	if _, err := e.externalDBConfig("sql_server"); err == nil || !strings.Contains(err.Error(), "DDCORE_SECRET_SQL_SERVER_DRIVER") {
		t.Fatalf("unsupported driver: %v", err)
	}
	t.Setenv("DDCORE_SECRET_SQL_SERVER_DRIVER", "SQLServer")
	if _, err := e.externalDBConfig("sql_server"); err != nil {
		t.Fatalf("driver name is case-insensitive: %v", err)
	}

	t.Setenv("DDCORE_SECRET_SQL_SERVER_USER", "")
	_, err = e.externalDBConfig("sql_server")
	var ce *cerr.Error
	if !errors.As(err, &ce) || ce.Type != "ValidationError" {
		t.Fatalf("missing user: %v", err)
	}
	if !strings.Contains(err.Error(), "DDCORE_SECRET_SQL_SERVER_USER") || strings.Contains(err.Error(), "p@ss") {
		t.Fatalf("the error must name the variable and not a value: %v", err)
	}
}

func TestExternalDBRedact(t *testing.T) {
	cfg := externalDBConfig{Password: "s3cr3t&x"}
	got := cfg.redact(errors.New("login failed: s3cr3t&x / s3cr3t%26x"))
	if strings.Contains(got, "s3cr3t") {
		t.Fatalf("password leaked: %s", got)
	}
}

func TestExternalDBs(t *testing.T) {
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, secretPrefix) {
			t.Setenv(k, "")
		}
	}
	setExternalEnv(t, "DDCORE_SECRET_ERP", map[string]string{
		"_HOST": "erp.local", "_DATABASE": "ERP", "_USER": "u", "_PASSWORD": "topsecret",
	})
	setExternalEnv(t, "DDCORE_SECRET_BROKEN", map[string]string{"_HOST": "b.local", "_DATABASE": "B"})
	// a lone _HOST is some other integration's secret, not a database
	t.Setenv("DDCORE_SECRET_SMTP_HOST", "mail.local")

	got := (&Engine{}).ExternalDBs()
	if len(got) != 2 || got[0].Name != "broken" || got[1].Name != "erp" {
		t.Fatalf("ExternalDBs: %+v", got)
	}
	if got[1].Host != "erp.local:1433" || got[1].Database != "ERP" || got[1].Error != "" {
		t.Fatalf("erp: %+v", got[1])
	}
	if !strings.Contains(got[0].Error, "DDCORE_SECRET_BROKEN_USER") {
		t.Fatalf("broken: %+v", got[0])
	}
	for _, x := range got {
		if strings.Contains(x.Error+x.Host+x.Database, "topsecret") {
			t.Fatalf("password leaked: %+v", x)
		}
	}
}

func TestNormalizeMSSQL(t *testing.T) {
	ts := time.Date(2026, 9, 22, 13, 4, 5, 0, time.UTC)
	guid := []byte{0xFF, 0x19, 0x96, 0x6F, 0x86, 0x8B, 0x11, 0xD0, 0xB4, 0x2D, 0x00, 0xC0, 0x4F, 0xC9, 0x64, 0xFF}
	for _, tc := range []struct {
		v    any
		typ  string
		want any
	}{
		{[]byte("1234.50"), "DECIMAL", 1234.5},
		{[]byte("-0.0001"), "NUMERIC", -0.0001},
		{[]byte("12.3400"), "MONEY", 12.34},
		{ts, "DATE", "2026-09-22"},
		{time.Date(1, 1, 1, 13, 4, 5, 0, time.UTC), "TIME", "13:04:05"},
		{ts, "DATETIME2", "2026-09-22T13:04:05Z"},
		{ts, "DATETIME", "2026-09-22T13:04:05Z"},
		{time.Date(2026, 9, 22, 13, 4, 5, 0, time.FixedZone("", -3*3600)), "DATETIMEOFFSET", "2026-09-22T13:04:05-03:00"},
		{guid, "UNIQUEIDENTIFIER", "6F9619FF-8B86-D011-B42D-00C04FC964FF"},
		{int64(42), "BIGINT", int64(42)},
		{int64(1 << 60), "BIGINT", "1152921504606846976"},
		{int64(-(1 << 60)), "BIGINT", "-1152921504606846976"},
		{int64(7), "INT", int64(7)},
		{"text", "NVARCHAR", "text"},
		{true, "BIT", true},
		{nil, "DECIMAL", nil},
	} {
		if got := normalizeMSSQL(tc.v, tc.typ); got != tc.want {
			t.Errorf("%s %v: got %#v, want %#v", tc.typ, tc.v, got, tc.want)
		}
	}
}

// The prefix gate refuses a write before any connection is attempted, which
// also means before any configuration is required.
func TestExternalSQLRefusesWrites(t *testing.T) {
	e := &Engine{}
	pool, err := js.NewPool(e, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	rt.Ctx = &Ctx{E: e, Ctx: context.Background()}
	_, err = rt.Eval(`ddcore.externalDb("nowhere").sql("UPDATE dbo.Documento SET ValorSaldo = 0")`)
	if err == nil || !strings.Contains(err.Error(), "only accepts SELECT") {
		t.Fatalf("a write was not refused: %v", err)
	}
	_, err = rt.Eval(`ddcore.externalDb("nowhere").sql("SELECT 1")`)
	if err == nil || !strings.Contains(err.Error(), "DDCORE_SECRET_NOWHERE_HOST") {
		t.Fatalf("a missing configuration was not named: %v", err)
	}
}

// TestExternalSQLIntegration runs against a real SQL Server when
// DDCORE_TEST_MSSQL_DSN (a sqlserver:// URL) is set.
func TestExternalSQLIntegration(t *testing.T) {
	dsn := os.Getenv("DDCORE_TEST_MSSQL_DSN")
	if dsn == "" {
		t.Skip("DDCORE_TEST_MSSQL_DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pw, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "1433"
	}
	database := u.Query().Get("database")
	if database == "" {
		database = "master"
	}
	setExternalEnv(t, "DDCORE_SECRET_MSSQL_IT", map[string]string{
		"_HOST": u.Hostname(), "_PORT": port, "_DATABASE": database,
		"_USER": u.User.Username(), "_PASSWORD": pw, "_ENCRYPT": u.Query().Get("encrypt"),
	})
	e := &Engine{}
	c := &Ctx{E: e, Ctx: context.Background()}
	if err := e.ExternalDBPing(context.Background(), "mssql_it"); err != nil {
		t.Fatal(err)
	}
	rows, err := c.ExternalSQL("mssql_it", `SELECT @@VERSION AS version, DB_NAME() AS db, SYSDATETIME() AS server_time,
		CAST(@p1 AS INT) + 1 AS next, CAST(12.5 AS DECIMAL(10,2)) AS amount, CAST('2026-09-22' AS DATE) AS day,
		CAST('6F9619FF-8B86-D011-B42D-00C04FC964FF' AS UNIQUEIDENTIFIER) AS guid`, []any{41}, 10)
	if err != nil {
		t.Fatal(err)
	}
	r := rows[0]
	if r["next"] != int64(42) || r["amount"] != 12.5 || r["day"] != "2026-09-22" ||
		r["guid"] != "6F9619FF-8B86-D011-B42D-00C04FC964FF" || r["db"] != database {
		t.Fatalf("row: %#v", r)
	}
	if _, ok := r["server_time"].(string); !ok {
		t.Fatalf("server_time: %#v", r["server_time"])
	}

	// the same call through the TS bridge
	pool, err := js.NewPool(e, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	rt.Ctx = c
	if _, err := rt.Eval(`const rows = ddcore.externalDb("mssql_it").sql("SELECT @p1 + 1 AS n, CAST(1.25 AS MONEY) AS m", [1], { timeout: 5 });
if (rows.length !== 1 || rows[0].n !== 2 || rows[0].m !== 1.25) throw new Error(JSON.stringify(rows));`); err != nil {
		t.Fatal(err)
	}
	// a write hidden behind WITH passes the prefix check but is never committed
	admin, err := sql.Open("sqlserver", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec("IF OBJECT_ID('dbo.ddcore_probe') IS NULL CREATE TABLE dbo.ddcore_probe (a INT); DELETE FROM dbo.ddcore_probe"); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP TABLE dbo.ddcore_probe")
	if _, err := c.ExternalSQL("mssql_it", "WITH x AS (SELECT 1 AS a) INSERT INTO dbo.ddcore_probe SELECT a FROM x", nil, 10); err != nil {
		t.Fatalf("the insert should run, and then be rolled back: %v", err)
	}
	var n int
	if err := admin.QueryRow("SELECT COUNT(*) FROM dbo.ddcore_probe").Scan(&n); err != nil || n != 0 {
		t.Fatalf("a write behind WITH was committed: %d rows, %v", n, err)
	}
	if _, err := c.ExternalSQL("mssql_it", "WAITFOR DELAY '00:00:05'", nil, 1); err == nil {
		t.Fatal("WAITFOR was accepted")
	}
	start := time.Now()
	_, err = c.ExternalSQL("mssql_it", "SELECT CHECKSUM_AGG(CHECKSUM(a.object_id, b.object_id, c.object_id)) AS n FROM sys.all_objects a CROSS JOIN sys.all_objects b CROSS JOIN sys.all_objects c", nil, 1)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("the timeout did not apply: %v after %s", err, time.Since(start))
	}
}
