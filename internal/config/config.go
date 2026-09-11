// Package config reads ddcore.json (and env overrides) from the site directory.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jrvidotti/ddcore/internal/num"
)

type File struct {
	DSN       string   `json:"dsn"`
	Apps      []string `json:"apps"`
	Port      int      `json:"port"`
	Workers   int      `json:"workers"`
	Scheduler bool     `json:"scheduler"`
	Site      string   `json:"site"`
	Lang      string   `json:"lang"`
	Currency  string   `json:"currency"`
	// CurrencyPrecision overrides how many decimal places a Currency field is
	// rounded to. Nil means "the currency's own minor unit" — 2 for USD, 0 for
	// JPY — which is right far more often than any number written here.
	CurrencyPrecision *int `json:"currencyPrecision"`
	// Rounding is "commercial" (half away from zero, the default) or "bankers"
	// (half to even).
	Rounding string `json:"rounding"`
	Timezone string `json:"timezone"`
	DataDir  string `json:"dataDir"`
	// ExportMaxRows caps GET /api/export so one download cannot hold a
	// connection and a worker for an unbounded time. 0 = DefaultExportMaxRows.
	ExportMaxRows int  `json:"exportMaxRows"`
	Dev           bool `json:"dev"`
}

const Name = "ddcore.json"

// Load reads ddcore.json from dir (or its parents) and applies env overrides.
func Load(dir string) (*File, string, error) {
	path, err := find(dir)
	f := &File{Port: 8080, Workers: 2, Lang: "pt-BR", Currency: "BRL", Timezone: "UTC", Site: "ddcore"}
	if err == nil {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(b, f); err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
	} else {
		path = filepath.Join(dir, Name)
	}
	if v := os.Getenv("DDCORE_DSN"); v != "" {
		f.DSN = v
	}
	if v := os.Getenv("DDCORE_PORT"); v != "" {
		f.Port, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("DATABASE_URL"); v != "" && f.DSN == "" {
		f.DSN = v
	}
	base := filepath.Dir(path)
	for i, a := range f.Apps {
		if !filepath.IsAbs(a) {
			f.Apps[i] = filepath.Join(base, a)
		}
	}
	if _, err := num.ParseRounding(f.Rounding); err != nil {
		// money is not a place to guess: a site that asks for a rounding rule
		// nobody implements must not quietly get a different one
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	if f.CurrencyPrecision != nil && (*f.CurrencyPrecision < 0 || *f.CurrencyPrecision > 9) {
		return nil, "", fmt.Errorf("%s: currencyPrecision must be between 0 and 9", path)
	}
	if f.DataDir == "" {
		f.DataDir = filepath.Join(base, "data")
	} else if !filepath.IsAbs(f.DataDir) {
		f.DataDir = filepath.Join(base, f.DataDir)
	}
	return f, path, nil
}

// RoundingMode is the site's rounding rule. Load already refused an
// unrecognised one, so there is nothing left to report here.
func (f *File) RoundingMode() num.Rounding {
	r, _ := num.ParseRounding(f.Rounding)
	return r
}

func find(dir string) (string, error) {
	d, _ := filepath.Abs(dir)
	for {
		p := filepath.Join(d, Name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", os.ErrNotExist
		}
		d = parent
	}
}

func (f *File) Save(path string) error {
	b, _ := json.MarshalIndent(f, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
