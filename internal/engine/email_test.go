package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  pessoa@example.com  "); got != "pessoa@example.com" {
		t.Fatalf("normalizeEmail()=%q", got)
	}
}

func TestValidEmail(t *testing.T) {
	valid := []string{
		"",
		"pessoa@example.com",
		"nome.sobrenome+tag@sub.example.com.br",
		"a@b.co",
	}
	for _, value := range valid {
		if !validEmail(value) {
			t.Errorf("validEmail(%q)=false", value)
		}
	}
}

func TestInvalidEmail(t *testing.T) {
	invalid := []string{
		"pessoa",
		"pessoa@localhost",
		"pessoa @example.com",
		"Pessoa <pessoa@example.com>",
		"a@example.com,b@example.com",
		".pessoa@example.com",
		"pessoa..teste@example.com",
		"pessoa@example..com",
		"pessoa@-example.com",
		"pessoa@example-.com",
		"pessoa@exemplo.cöm",
		string(make([]byte, 65)) + "@example.com",
	}
	for _, value := range invalid {
		if validEmail(value) {
			t.Errorf("validEmail(%q)=true", value)
		}
	}
}

func TestCastEmailNormalizesWhitespace(t *testing.T) {
	field := &meta.Field{Fieldname: "email", Fieldtype: "Email", Label: "E-mail"}
	got, err := castValueWith(field, "  pessoa@example.com  ", utcOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pessoa@example.com" {
		t.Fatalf("castValueWith()=%q", got)
	}
}

func TestCastEmailTurnsWhitespaceIntoEmptyOptionalValue(t *testing.T) {
	field := &meta.Field{Fieldname: "email", Fieldtype: "Email", Label: "E-mail"}
	got, err := castValueWith(field, "   ", utcOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("castValueWith()=%#v want nil", got)
	}
}

func TestCheckEmailsRejectsInvalidValue(t *testing.T) {
	c := testCtxWithI18n()
	d := &meta.DocType{Name: "Contact", Fields: []*meta.Field{{Fieldname: "email", Fieldtype: "Email", Label: "E-mail"}}}
	err := c.checkEmails(d, Doc{"email": "invalid"})
	if err == nil {
		t.Fatal("expected invalid email error")
	}
	ce := cerr.From(err)
	if ce.Type != "ValidationError" || ce.Title != "Invalid email" || !strings.Contains(ce.Message, "E-mail") {
		t.Fatalf("unexpected error: %#v", ce)
	}
}

func TestCheckEmailsAllowsEmptyAndSystemUsers(t *testing.T) {
	c := testCtxWithI18n()
	contact := &meta.DocType{Name: "Contact", Fields: []*meta.Field{{Fieldname: "email", Fieldtype: "Email", Label: "E-mail"}}}
	if err := c.checkEmails(contact, Doc{"email": nil}); err != nil {
		t.Fatalf("optional empty email: %v", err)
	}
	user := &meta.DocType{Name: "User", Fields: []*meta.Field{{Fieldname: "email", Fieldtype: "Email", Label: "E-mail"}}}
	for _, value := range []string{"Administrator", "Guest"} {
		if err := c.checkEmails(user, Doc{"email": value}); err != nil {
			t.Fatalf("system user %q: %v", value, err)
		}
	}
}

// testCtxWithI18n builds a Ctx with an empty catalogue: checkEmails only needs T().
func testCtxWithI18n() *Ctx {
	st := &State{I18n: &I18n{dict: map[string]map[string]string{}}}
	return &Ctx{E: &Engine{State: st}, St: st}
}
