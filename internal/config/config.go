// Package config reads the site's configuration from two files that answer two
// different questions.
//
// ddcore.json is what the site decided: its currency and precision, its
// timezone, its apps, its access policy. Those are the same on a laptop, in
// staging and in production, so the file is committed and a change to it is a
// change to the product.
//
// .env is where the site is running: the database, the port, the public
// address, whether a proxy sits in front, how mail leaves. Those differ on
// every machine and one of them is a password, so the file is gitignored and
// .env.example is the committed record of which variables exist.
//
// Precedence runs from the most specific outwards: a variable already in the
// real environment beats .env, which beats ddcore.json, which beats the
// default. That order is what lets a platform — Railway injecting
// DATABASE_URL, `docker run -e` — override a file baked into the image
// without anyone editing it.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/num"
)

type File struct {
	DSN       string   `json:"dsn"`
	Apps      []string `json:"apps"`
	Port      int      `json:"port"`
	Workers   int      `json:"workers"`
	Scheduler bool     `json:"scheduler"`
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
	ExportMaxRows int `json:"exportMaxRows"`
	// ImportMaxRows caps the rows of one Data Import upload, which is written
	// while the request waits. 0 = DefaultImportMaxRows.
	ImportMaxRows int  `json:"importMaxRows"`
	Dev           bool `json:"dev"`
	// Auth is the access policy: session life, lockout, token expiry.
	Auth AuthPolicy `json:"auth"`
	// Ops is the operational policy: the thresholds a health report and
	// `ddcore doctor` call a warning.
	Ops OpsPolicy `json:"ops"`
	// URL is the site's public base address. Recovery and invitation links are
	// built from it, so a wrong one is worse than no mail at all.
	URL string `json:"url"`
	// TrustProxy makes the server believe X-Forwarded-For. Only turn it on
	// when a proxy you control actually sets it: otherwise every client can
	// claim any address, and the throttle that keys on the address becomes a
	// way to lock out a stranger.
	TrustProxy bool `json:"trustProxy"`
	// Login is what the sign-in screen tells a visitor before they sign in.
	Login LoginPage `json:"login"`
	// Mail comes from the environment only — see the package comment.
	Mail Mail `json:"-"`
	// Webhooks comes from the environment only, like Mail.
	Webhooks Webhooks `json:"-"`
	// UpdateCheck comes from the environment only, like Webhooks.
	UpdateCheck UpdateCheck `json:"-"`
	// Storage comes from the environment only, like Mail.
	Storage Storage `json:"-"`
	// OIDC lists the single sign-on providers, from the environment only.
	OIDC []OIDCProvider `json:"-"`
	// Backup comes from the environment only, like Storage, whose bucket it
	// borrows when it is given none of its own.
	Backup Backup `json:"-"`
}

// LoginPage is served by the public /api/boot to anyone who opens the site,
// signed in or not. It is meant for a public demo or a maintenance note: never
// put a real account's password here.
type LoginPage struct {
	// Notice is plain text, shown as written — it is the operator's sentence,
	// not a catalogue key, so it is not translated. A literal \n in the
	// environment variable is a line break.
	Notice string `json:"notice"`
	// DemoUser and DemoPassword offer a button that fills in the form. Both
	// or neither: one alone is dropped.
	DemoUser     string `json:"demoUser"`
	DemoPassword string `json:"demoPassword"`
}

const Name = "ddcore.json"

// Default is the configuration a site gets when ddcore.json says nothing. Load
// starts from it and `ddcore init` writes it, so a freshly created file loads
// as written: a zero in a policy block is a refusal, not "use the default".
func Default() *File {
	return &File{Apps: []string{}, Port: 8080, Workers: 2, Lang: "en", Currency: "USD",
		Timezone: "UTC", Auth: DefaultAuth(), Ops: DefaultOps()}
}

// Load reads ddcore.json from dir (or its parents) and applies env overrides.
func Load(dir string) (*File, string, error) {
	path, err := find(dir)
	f := Default()
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
	// .env sits next to ddcore.json, and is read after it so the environment
	// reads as the outer layer — but it never overwrites a variable the real
	// environment already set.
	if err := loadDotenv(filepath.Join(filepath.Dir(path), DotenvName)); err != nil {
		return nil, "", fmt.Errorf("%s: %w", DotenvName, err)
	}
	// DATABASE_URL from the platform overrides ddcore.json,
	// and explicit DDCORE_DSN overrides DATABASE_URL.
	if v := os.Getenv("DATABASE_URL"); v != "" {
		f.DSN = v
	}
	if v := os.Getenv("DDCORE_DSN"); v != "" {
		f.DSN = v
	}
	if v := env("DDCORE_PORT", os.Getenv("PORT")); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			f.Port = p
		}
	}
	f.URL = strings.TrimSuffix(env("DDCORE_URL", f.URL), "/")
	f.TrustProxy = envBool("DDCORE_TRUST_PROXY", f.TrustProxy)
	f.Login.Notice = strings.ReplaceAll(env("DDCORE_LOGIN_NOTICE", f.Login.Notice), `\n`, "\n")
	f.Login.DemoUser = env("DDCORE_LOGIN_DEMO_USER", f.Login.DemoUser)
	f.Login.DemoPassword = env("DDCORE_LOGIN_DEMO_PASSWORD", f.Login.DemoPassword)
	if f.Login.DemoUser == "" || f.Login.DemoPassword == "" {
		// half an account fills half the form, which only teaches the visitor
		// that the offer does not work
		f.Login.DemoUser, f.Login.DemoPassword = "", ""
	}
	if f.Mail, err = mailFromEnv(); err != nil {
		return nil, "", err
	}
	f.Mail.Dev = f.Dev || envBool("DDCORE_DEV", false)
	if f.Webhooks, err = webhooksFromEnv(); err != nil {
		return nil, "", err
	}
	if f.UpdateCheck, err = updateCheckFromEnv(); err != nil {
		return nil, "", err
	}
	if f.Storage, err = storageFromEnv(); err != nil {
		return nil, "", err
	}
	if f.OIDC, err = oidcFromEnv(); err != nil {
		return nil, "", err
	}
	if f.Backup, err = backupFromEnv(f.Storage); err != nil {
		return nil, "", err
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
	if err := f.Auth.validate(); err != nil {
		// access policy is not a place to guess either: a site that asks for a
		// lockout nobody implements must not quietly run without one
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	if err := f.validateOIDC(); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	// A partial `ops` block leaves the fields it omits at zero, and a zero
	// threshold is not "no threshold": it is an alarm that fires on the first
	// job. Fill the gaps before validating what is left.
	f.Ops = f.Ops.WithDefaults()
	if err := f.Ops.validate(); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	// DDCORE_DATA_DIR moves uploads and local backups per deployment — a
	// restore drill beside production needs a data directory of its own
	f.DataDir = env("DDCORE_DATA_DIR", f.DataDir)
	if f.DataDir == "" {
		f.DataDir = filepath.Join(base, "data")
	} else if !filepath.IsAbs(f.DataDir) {
		f.DataDir = filepath.Join(base, f.DataDir)
	}
	if f.Backup.Dir == "" {
		f.Backup.Dir = filepath.Join(f.DataDir, "backups")
	} else if !filepath.IsAbs(f.Backup.Dir) {
		f.Backup.Dir = filepath.Join(base, f.Backup.Dir)
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

// Edit changes ddcore.json in dir (or its parents) through fn and saves it
// when fn reports a change. It works on the file as written, not on what Load
// resolves: a DSN from .env or an app directory made absolute must never end
// up in the committed file. A result that Load would refuse is not saved.
func Edit(dir string, fn func(f *File, path string) bool) (string, error) {
	path, err := find(dir)
	if err != nil {
		return "", fmt.Errorf("%s not found (run `ddcore init`): %w", Name, err)
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	f := Default()
	if err := json.Unmarshal(orig, f); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if !fn(f, path) {
		return path, nil
	}
	if err := f.Save(path); err != nil {
		return "", err
	}
	if _, _, err := Load(filepath.Dir(path)); err != nil {
		_ = os.WriteFile(path, orig, 0o644)
		return "", err
	}
	return path, nil
}

func (f *File) Save(path string) error {
	b, _ := json.MarshalIndent(f, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// PublicURL is the base a link mailed to a person must use. An empty url falls
// back to localhost so development works, and HasPublicURL is how the caller
// knows to say so out loud — a recovery link pointing at localhost is a
// support ticket waiting to happen.
func (f *File) PublicURL() string {
	if f.URL != "" {
		return f.URL
	}
	return fmt.Sprintf("http://localhost:%d", f.Port)
}

func (f *File) HasPublicURL() bool { return f.URL != "" }
