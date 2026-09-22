package engine

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomID() string {
	b := make([]byte, 10)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

var hashRun = regexp.MustCompile(`#+`)

// setID decides the id of a new document following its idGeneration rule.
func (c *Ctx) setID(d *meta.DocType, doc Doc) error {
	if d.IsSingle {
		doc["id"] = "singleton"
		return nil
	}
	g := d.IDGeneration
	switch {
	case doc.Str("amended_from") != "" && doc.Str("id") != "":
		// amended documents keep "<original>-<n>"
	case g.Series != "":
		series := g.Series
		if doc.Str("id_series") != "" {
			series = doc.Str("id_series")
		}
		id, err := c.nextInSeries(series, doc)
		if err != nil {
			return err
		}
		doc["id"] = id
	case g.Field != "":
		v := strings.TrimSpace(doc.Str(g.Field))
		if v == "" {
			return cerr.Mandatory("{0} is required to identify the document", c.T(d.Field(g.Field).Label))
		}
		doc["id"] = v
	case g.Format != "":
		id, err := c.formatID(g.Format, doc)
		if err != nil {
			return err
		}
		doc["id"] = id
	case g.Prompt:
		if strings.TrimSpace(doc.Str("id")) == "" {
			return cerr.Mandatory("Provide the document ID")
		}
	default:
		if strings.TrimSpace(doc.Str("id")) == "" {
			doc["id"] = randomID()
		}
	}
	doc["id"] = strings.TrimSpace(doc.Str("id"))
	if doc.Str("id") == "" {
		doc["id"] = randomID()
	}
	if ok, _ := c.idExists(d.Name, doc.Str("id")); ok {
		return cerr.Duplicate("{0} {1} already exists", c.T(d.Label), doc.Str("id")).WithTitleKey("Duplicate ID")
	}
	return nil
}

// nextInSeries renders a Frappe-style series like "CTR-.YYYY.-.####".
func (c *Ctx) nextInSeries(series string, doc Doc) (string, error) {
	parts := strings.Split(series, ".")
	now := time.Now()
	var prefix strings.Builder
	digits := 0
	for _, p := range parts {
		switch {
		case p == "":
			continue
		case hashRun.MatchString(p) && strings.Trim(p, "#") == "":
			digits = len(p)
		case p == "YYYY":
			prefix.WriteString(now.Format("2006"))
		case p == "YY":
			prefix.WriteString(now.Format("06"))
		case p == "MM":
			prefix.WriteString(now.Format("01"))
		case p == "DD":
			prefix.WriteString(now.Format("02"))
		case strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}"):
			prefix.WriteString(doc.Str(strings.Trim(p, "{}")))
		default:
			prefix.WriteString(p)
		}
	}
	if digits == 0 {
		digits = 5
	}
	n, err := c.nextCounter(prefix.String())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%0*d", prefix.String(), digits, n), nil
}

func (c *Ctx) nextCounter(key string) (int64, error) {
	var n int64
	err := c.Q().QueryRow(c.Ctx, `INSERT INTO ddcore_series (prefix, current) VALUES ($1, 1)
		ON CONFLICT (prefix) DO UPDATE SET current = ddcore_series.current + 1 RETURNING current`, key).Scan(&n)
	return n, err
}

var fmtField = regexp.MustCompile(`\{([a-zA-Z_#]+)\}`)

// formatID renders "{imovel}-{YYYY}-{###}".
func (c *Ctx) formatID(format string, doc Doc) (string, error) {
	now := time.Now()
	counter := 0
	out := fmtField.ReplaceAllStringFunc(format, func(m string) string {
		k := strings.Trim(m, "{}")
		switch {
		case k == "YYYY":
			return now.Format("2006")
		case k == "YY":
			return now.Format("06")
		case k == "MM":
			return now.Format("01")
		case k == "DD":
			return now.Format("02")
		case strings.Trim(k, "#") == "":
			counter = len(k)
			return "\x00"
		}
		return doc.Str(k)
	})
	if counter > 0 {
		prefix := strings.ReplaceAll(out, "\x00", "")
		n, err := c.nextCounter(prefix)
		if err != nil {
			return "", err
		}
		out = strings.ReplaceAll(out, "\x00", fmt.Sprintf("%0*d", counter, n))
	}
	return out, nil
}

// SeriesOptions returns the series prefixes a doctype allows.
func SeriesOptions(d *meta.DocType) []string {
	if f := d.Field("id_series"); f != nil {
		return f.SelectValues()
	}
	if d.IDGeneration.Series != "" {
		return []string{d.IDGeneration.Series}
	}
	return nil
}

var _ = db.Str
