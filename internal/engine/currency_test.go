package engine

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
)

func money(prec int, r num.Rounding) castOpts {
	return castOpts{loc: utcOpts.loc, currencyPrec: prec, rounding: r}
}

// The precision a Currency field is rounded to resolves currency → site →
// field. Storing more decimals than the site declares is what makes a form
// show 10.01 while the database sums 10.005.
func TestCurrencyRoundsToTheResolvedPrecision(t *testing.T) {
	for _, c := range []struct {
		why   string
		field *meta.Field
		opts  castOpts
		in    any
		want  float64
	}{
		{"the site's currency decides", &meta.Field{Fieldtype: "Currency", Label: "Total"}, money(2, num.HalfAwayFromZero), 10.005, 10.01},
		{"a JPY site has no minor unit at all", &meta.Field{Fieldtype: "Currency", Label: "Total"}, money(0, num.HalfAwayFromZero), 1234.5, 1235},
		{"and rounds the other way when asked to", &meta.Field{Fieldtype: "Currency", Label: "Total"}, money(0, num.HalfToEven), 1234.5, 1234},
		{"three places, as a KWD site", &meta.Field{Fieldtype: "Currency", Label: "Total"}, money(3, num.HalfAwayFromZero), 1.23456, 1.235},
		{"the field overrides the site", &meta.Field{Fieldtype: "Currency", Label: "Rate", Precision: 4}, money(2, num.HalfAwayFromZero), 1.234567, 1.2346},
		{"a string from an import is coerced first", &meta.Field{Fieldtype: "Currency", Label: "Total"}, money(2, num.HalfAwayFromZero), "10.005", 10.01},
		{"a Percent is a rate, not an amount, and keeps its decimals", &meta.Field{Fieldtype: "Percent", Label: "Share"}, money(2, num.HalfAwayFromZero), 33.333333, 33.333333},
		{"a Float is a measurement and is never rounded", &meta.Field{Fieldtype: "Float", Label: "Weight", Precision: 2}, money(2, num.HalfAwayFromZero), 1.23456789, 1.23456789},
		{"an Int keeps its own rule, not the site's money rule", &meta.Field{Fieldtype: "Int", Label: "Qty"}, money(2, num.HalfToEven), 2.5, 3},
	} {
		got, err := castValueWith(c.field, c.in, c.opts)
		if err != nil {
			t.Fatalf("%s: %v", c.why, err)
		}
		var f float64
		switch x := got.(type) {
		case float64:
			f = x
		case int64:
			f = float64(x)
		default:
			t.Fatalf("%s: got %#v", c.why, got)
		}
		if f != c.want {
			t.Errorf("%s: %v → %v, want %v", c.why, c.in, f, c.want)
		}
	}
}

// Rounding on write must not make an old row look edited. Both sides of the
// read-only and allow-on-submit comparisons go through the same coercion, so a
// value stored before this contract existed still compares equal to itself.
func TestRoundingIsIdempotentAcrossSaves(t *testing.T) {
	f := &meta.Field{Fieldtype: "Currency", Label: "Total"}
	first, err := castValueWith(f, 10.005, money(2, num.HalfAwayFromZero))
	if err != nil {
		t.Fatal(err)
	}
	second, err := castValueWith(f, first, money(2, num.HalfAwayFromZero))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("re-coercing moved the value: %v then %v", first, second)
	}
}

// A number a column cannot hold used to reach Postgres and come back as a
// driver error naming neither the field nor the value — or, for NaN, as a
// silent null.
func TestNumericRefusesWhatAColumnCannotHold(t *testing.T) {
	for _, c := range []struct {
		ft string
		v  any
	}{
		{"Currency", math.NaN()},
		{"Currency", math.Inf(1)},
		{"Currency", math.Inf(-1)},
		{"Currency", 1e13},
		{"Currency", -1e13},
		{"Percent", 1e12},
		{"Float", math.NaN()},
		{"Int", math.Inf(1)},
	} {
		f := &meta.Field{Fieldtype: c.ft, Label: "Valor"}
		if _, err := castValueWith(f, c.v, money(2, num.HalfAwayFromZero)); err == nil {
			t.Errorf("%s accepted %v", c.ft, c.v)
		} else if got := cerr.From(err).Type; got != "ValidationError" {
			t.Errorf("%s %v gave %s, want ValidationError", c.ft, c.v, got)
		}
	}
}

func moneyApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "money", title: "Money", roles: ["Gestor"] });`)
	w("doctypes/parcela/parcela.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Parcela", isChild: true, fields: [
  { fieldname: "vencimento", fieldtype: "Date", label: "Vencimento" },
  { fieldname: "valor", fieldtype: "Currency", label: "Valor" },
] });`)
	w("doctypes/fatura/fatura.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Fatura", fields: [
  { fieldname: "emissao", fieldtype: "Date", label: "Emissão" },
  { fieldname: "total", fieldtype: "Currency", label: "Total" },
  { fieldname: "taxa", fieldtype: "Percent", label: "Taxa" },
  { fieldname: "peso", fieldtype: "Float", label: "Peso" },
  { fieldname: "parcelas", fieldtype: "Table", label: "Parcelas", options: "Parcela" },
], permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`)
	return dir
}

func setupMoney(t *testing.T, cfg Config) *Engine {
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres indisponível em DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres indisponível: %v", err)
	}
	e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	e0.DB.Close()
	cfg.DSN, cfg.Apps, cfg.Test = testDSN, []js.App{{Name: "money", Dir: moneyApp(t)}}, true
	e, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	return e
}

// The end of the contract: what the column actually holds. numeric(21,9) will
// happily store 10.005000000, so the proof has to read the stored value back,
// not the document the test just built.
func TestCurrencyReachesTheColumnRounded(t *testing.T) {
	e := setupMoney(t, Config{Currency: "USD"})
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		d, _ := c.NewDoc("Fatura", Doc{
			"emissao": "2018-11-04", // Brazil's DST start: local midnight did not exist
			"total":   10.005,
			"taxa":    33.333333,
			"peso":    1.23456789,
			"parcelas": []any{
				// a total split three ways, with the residue cent placed
				// deliberately — 33.334/33.333/33.333 would each round down
				// and the invoice would be a cent short of itself
				map[string]any{"vencimento": "2018-12-04", "valor": 33.34},
				map[string]any{"vencimento": "2019-01-04", "valor": 33.33},
				map[string]any{"vencimento": "2019-02-04", "valor": 33.33},
			},
		})
		saved, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		rows, err := c.SQL(`SELECT total::text AS total, taxa::text AS taxa, peso, emissao::text AS emissao,
			(SELECT sum(valor) FROM tab_parcela WHERE parent = f.name)::text AS soma
			FROM tab_fatura f WHERE name = $1`, []any{saved.Name()})
		if err != nil {
			return err
		}
		got := rows[0]
		if got["total"] != "10.010000000" {
			t.Errorf("total stored as %v, want 10.010000000", got["total"])
		}
		if got["taxa"] != "33.333333000" {
			t.Errorf("taxa stored as %v, want 33.333333000 — a Percent is a rate and must keep its decimals", got["taxa"])
		}
		if got["peso"] != 1.23456789 {
			t.Errorf("peso stored as %v, want 1.23456789 — a Float is a measurement", got["peso"])
		}
		if got["emissao"] != "2018-11-04" {
			t.Errorf("emissao stored as %v: a civil date must not slide", got["emissao"])
		}
		if got["soma"] != "100.000000000" {
			t.Errorf("the instalments sum to %v, want 100.000000000", got["soma"])
		}

		// saving the document again must not look like an edit
		reread, err := c.GetDoc("Fatura", saved.Name())
		if err != nil {
			return err
		}
		if _, err := c.Save(reread, SaveOpts{IgnorePermissions: true}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The reason splitAmount exists: an even-looking split of a total does not add
// back up once each part is rounded, and the shortfall is silent.
func TestNaiveThirdsDoNotAddUp(t *testing.T) {
	f := &meta.Field{Fieldtype: "Currency", Label: "Valor"}
	sum := 0.0
	for i := 0; i < 3; i++ {
		v, err := castValueWith(f, 100.0/3, money(2, num.HalfAwayFromZero))
		if err != nil {
			t.Fatal(err)
		}
		sum += v.(float64)
	}
	if sum == 100 {
		t.Fatal("three rounded thirds added up to the whole; the premise of splitAmount is gone")
	}
	if sum != 99.99 {
		t.Fatalf("three rounded thirds = %v, want 99.99", sum)
	}
}

// The same document on a site with no cents and the other rounding rule. The
// pure tests cover the matrix; this one proves the settings actually reach the
// column, which is the only place the answer counts.
func TestAJapaneseSiteStoresWholeYen(t *testing.T) {
	e := setupMoney(t, Config{Currency: "JPY", Rounding: num.HalfToEven})
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		d, _ := c.NewDoc("Fatura", Doc{"emissao": "2026-03-01", "total": 1234.5})
		saved, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		rows, err := c.SQL(`SELECT total::text AS total FROM tab_fatura WHERE name = $1`, []any{saved.Name()})
		if err != nil {
			return err
		}
		// 1234.5 is an exact half: banker's rounding takes it down to the even
		// 1234, where the commercial rule would give 1235
		if rows[0]["total"] != "1234.000000000" {
			t.Errorf("total stored as %v, want 1234.000000000", rows[0]["total"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A JPY site has no cents, and the site's own currency is what says so.
func TestCurrencyPrecisionFollowsTheSiteCurrency(t *testing.T) {
	for _, c := range []struct {
		cfg  Config
		want int
	}{
		{Config{Currency: "USD"}, 2},
		{Config{Currency: "JPY"}, 0},
		{Config{Currency: "KWD"}, 3},
		{Config{Currency: "JPY", CurrencyPrecision: ptr(4)}, 4},
	} {
		e := &Engine{Cfg: c.cfg}
		if got := e.CurrencyPrecision(); got != c.want {
			t.Errorf("%+v: CurrencyPrecision() = %d, want %d", c.cfg, got, c.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }
