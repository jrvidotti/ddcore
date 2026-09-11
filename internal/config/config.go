// Package config reads ddcore.json (and env overrides) from the site directory.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	Timezone  string   `json:"timezone"`
	DataDir   string   `json:"dataDir"`
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
	if f.DataDir == "" {
		f.DataDir = filepath.Join(base, "data")
	} else if !filepath.IsAbs(f.DataDir) {
		f.DataDir = filepath.Join(base, f.DataDir)
	}
	return f, path, nil
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
