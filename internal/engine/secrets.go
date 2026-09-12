package engine

import (
	"os"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// The framework's answer to "where does an integration credential live" is:
// not here. Not in a column, not in a document, not in a fixture.
//
// A secret kept in the database is a secret in every backup, every export,
// every replica and every Version diff, and keeping it out of those is a list
// of places to remember rather than a property of the system. A secret kept in
// the environment is in none of them by construction: nothing reads it but the
// process that needs it, and rotating it is a redeploy rather than a migration.
//
// So an app asks for `ddcore.secret("stripe_key")` and the framework reads
// DDCORE_SECRET_STRIPE_KEY, which .env supplies in development and the
// platform supplies in production.

// secretPrefix namespaces what an app may read. Without it `ddcore.secret`
// would be a window onto the whole environment — the DSN, the SMTP password,
// anything else the process was started with — and an app could read a
// credential that is not its own.
const secretPrefix = "DDCORE_SECRET_"

// SecretEnvName is the variable that holds a given secret. It is exported so
// `ddcore doctor` and an error message can name the variable someone has to
// set, instead of making them guess the spelling.
func SecretEnvName(name string) string {
	var b strings.Builder
	b.WriteString(secretPrefix)
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32)
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Secret returns the value of a named secret, and whether it was set at all.
func (e *Engine) Secret(name string) (string, bool) {
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	v, ok := os.LookupEnv(SecretEnvName(name))
	return v, ok && v != ""
}

// RequireSecret is Secret for the caller that cannot continue without it. The
// error names the variable but never the value — an Error Log row and a desk
// toast are both places a secret must not turn up.
func (e *Engine) RequireSecret(name string) (string, error) {
	v, ok := e.Secret(name)
	if !ok {
		return "", cerr.Validation("Secret {0} is not configured on this site", SecretEnvName(name))
	}
	return v, nil
}

// SecretNames lists the secrets this process was given, values omitted. It is
// what lets `ddcore doctor` say "this one is missing" without printing any.
func (e *Engine) SecretNames() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, secretPrefix) && v != "" {
			out = append(out, strings.TrimPrefix(k, secretPrefix))
		}
	}
	return out
}

// RedactPassword blanks every Password field in a document on its way out of
// the API.
//
// A Password column is still plain text at rest — that is the honest state of
// things — but it already stays out of Version and out of an export, and this
// closes the last way it left the server: a read by anyone the document's
// permissions allow. A controller keeps seeing the real value, because it
// works on the Ctx and never through this border.
func RedactPassword(d *meta.DocType, doc Doc) {
	if d == nil || doc == nil {
		return
	}
	for _, f := range d.Fields {
		if f.Fieldtype == "Password" && doc[f.Fieldname] != nil {
			doc[f.Fieldname] = nil
		}
		if isSecretField(f.Fieldname) && doc[f.Fieldname] != nil {
			doc[f.Fieldname] = nil
		}
	}
}

func (c *Ctx) redactVault(d *meta.DocType, doc Doc) {
	if d == nil || doc == nil || doc.Name() == "" {
		return
	}
	for _, f := range d.Fields {
		if f.Fieldtype == "Vault" {
			key := c.DeriveVaultKey(d, f, doc)
			has, _ := c.E.VaultHas(c.Ctx, c.Q(), key)
			if has {
				doc[f.Fieldname] = map[string]any{"configured": true}
			} else {
				doc[f.Fieldname] = nil
			}
		}
	}
}

// RedactDoc looks the doctype up and redacts, children included.
func (c *Ctx) RedactDoc(doctype string, doc Doc) Doc {
	if doc == nil {
		return doc
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return doc
	}
	RedactPassword(d, doc)
	c.redactVault(d, doc)
	for _, f := range d.Fields {
		if f.Fieldtype != "Table" || f.OptionsString() == "" {
			continue
		}
		cd, err := c.St.DocType(f.OptionsString())
		if err != nil {
			continue
		}
		for _, row := range doc.Children(f.Fieldname) {
			RedactPassword(cd, row)
			c.redactVault(cd, row)
		}
	}
	return doc
}
