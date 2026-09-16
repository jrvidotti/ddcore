package api

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/print"
)

func TestPrintAPI_FormatsAndRender(t *testing.T) {
	x := setup(t)

	// 1. Insert a Pessoa document as Admin
	x.asAdmin(func(c *engine.Ctx) error {
		p, err := c.NewDoc("Pessoa", engine.Doc{
			"nome": "João da Silva",
			"tipo": "PF",
			"contatos": []any{
				map[string]any{"telefone": "11999998888"},
			},
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, engine.SaveOpts{})
		return err
	})

	anaAuth := "sid:" + x.sid("ana@x.com")
	zeAuth := "sid:" + x.sid("ze@x.com")

	// 2. Test GET /api/print/formats/Pessoa
	rFormats := x.call(http.MethodGet, "/api/print/formats/Pessoa", nil, anaAuth)
	if rFormats.Status != http.StatusOK {
		t.Fatalf("expected 200 for formats, got %d: %s", rFormats.Status, rFormats.Raw)
	}
	if !strings.Contains(rFormats.Raw, "standard") {
		t.Fatalf("expected standard format in response: %s", rFormats.Raw)
	}

	// 3. Test GET /api/print/Pessoa/João%20da%20Silva as authorized user (ana)
	rPrint := x.call(http.MethodGet, "/api/print/Pessoa/Jo%C3%A3o%20da%20Silva", nil, anaAuth)
	if rPrint.Status != http.StatusOK {
		t.Fatalf("expected 200 for print, got %d: %s", rPrint.Status, rPrint.Raw)
	}
	if ct := rPrint.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html Content-Type, got: %s", ct)
	}
	if !strings.Contains(rPrint.Raw, "João da Silva") {
		t.Fatalf("expected document content in HTML output: %s", rPrint.Raw)
	}
	if !strings.Contains(rPrint.Raw, "11999998888") {
		t.Fatalf("expected child table row in HTML output: %s", rPrint.Raw)
	}

	// 4. Security: Test GET /api/print/Pessoa/... as unauthorized user (ze)
	rUnauthorized := x.call(http.MethodGet, "/api/print/Pessoa/Jo%C3%A3o%20da%20Silva", nil, zeAuth)
	if rUnauthorized.Status != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized user, got %d: %s", rUnauthorized.Status, rUnauthorized.Raw)
	}
}

func TestPrintAPI_LetterheadsAndPDF(t *testing.T) {
	x := setup(t)

	// 1. Insert Letter Head and Pessoa
	x.asAdmin(func(c *engine.Ctx) error {
		lh, err := c.NewDoc("Letter Head", engine.Doc{
			"letter_head_name": "Empresa Teste",
			"header_html":      "<div class='lh-hdr'>Header Teste</div>",
			"footer_html":      "<div class='lh-ftr'>Footer Teste</div>",
			"is_default":       true,
		})
		if err != nil {
			return err
		}
		if _, err := c.Insert(lh, engine.SaveOpts{}); err != nil {
			return err
		}

		p, err := c.NewDoc("Pessoa", engine.Doc{
			"nome": "Maria Santos",
			"tipo": "PJ",
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, engine.SaveOpts{})
		return err
	})

	anaAuth := "sid:" + x.sid("ana@x.com")

	// 2. Test GET /api/letterheads
	rLH := x.call(http.MethodGet, "/api/letterheads", nil, anaAuth)
	if rLH.Status != http.StatusOK {
		t.Fatalf("expected 200 for letterheads, got %d: %s", rLH.Status, rLH.Raw)
	}
	if !strings.Contains(rLH.Raw, "Empresa Teste") {
		t.Fatalf("expected Empresa Teste in letterheads: %s", rLH.Raw)
	}

	// 3. Test GET /api/print/Pessoa/Maria%20Santos includes default letterhead
	rPrintLH := x.call(http.MethodGet, "/api/print/Pessoa/Maria%20Santos", nil, anaAuth)
	if rPrintLH.Status != http.StatusOK {
		t.Fatalf("expected 200 for print, got %d: %s", rPrintLH.Status, rPrintLH.Raw)
	}
	if !strings.Contains(rPrintLH.Raw, "Header Teste") || !strings.Contains(rPrintLH.Raw, "Footer Teste") {
		t.Fatalf("expected letterhead header and footer in HTML output: %s", rPrintLH.Raw)
	}

	// 4. Test PDF endpoint with custom mock command
	print.ResetDefaultRenderer()
	defer print.ResetDefaultRenderer()

	os.Setenv("DDCORE_PDF_COMMAND", `echo '%PDF-1.4 api test' > {out}`)
	defer os.Unsetenv("DDCORE_PDF_COMMAND")

	rPDF := x.call(http.MethodGet, "/api/print/Pessoa/Maria%20Santos/pdf?download=1", nil, anaAuth)
	if rPDF.Status != http.StatusOK {
		t.Fatalf("expected 200 for PDF, got %d: %s", rPDF.Status, rPDF.Raw)
	}
	if ct := rPDF.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("expected application/pdf, got: %s", ct)
	}
	if !strings.Contains(rPDF.Raw, "%PDF-1.4 api test") {
		t.Fatalf("expected PDF output, got: %s", rPDF.Raw)
	}
	cd := rPDF.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "Pessoa-Maria Santos.pdf") {
		t.Fatalf("expected attachment Content-Disposition, got: %s", cd)
	}
}

// page_format and landscape shape the @page rule of the HTML as well as the
// PDF; an unsupported page format is refused rather than ignored.
func TestPrintAPI_PageFormat(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		p, err := c.NewDoc("Pessoa", engine.Doc{"nome": "Paula", "tipo": "PF"})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, engine.SaveOpts{})
		return err
	})
	anaAuth := "sid:" + x.sid("ana@x.com")

	r := x.call(http.MethodGet, "/api/print/Pessoa/Paula", nil, anaAuth)
	x.expect(r, http.StatusOK, "")
	if !strings.Contains(r.Raw, "size: A4 portrait;") {
		t.Fatalf("expected A4 portrait by default: %s", r.Raw)
	}
	r = x.call(http.MethodGet, "/api/print/Pessoa/Paula?page_format=Letter&landscape=1", nil, anaAuth)
	x.expect(r, http.StatusOK, "")
	if !strings.Contains(r.Raw, "size: Letter landscape;") {
		t.Fatalf("expected Letter landscape: %s", r.Raw)
	}
	x.expect(x.call(http.MethodGet, "/api/print/Pessoa/Paula?page_format=Legal", nil, anaAuth), http.StatusExpectationFailed, "ValidationError")
	x.expect(x.call(http.MethodGet, "/api/print/Pessoa/Paula/pdf?page_format=Legal", nil, anaAuth), http.StatusExpectationFailed, "ValidationError")
}

// The format and letterhead lists answer signed-in users only, and the format
// list also needs read permission on the DocType.
func TestPrintAPI_ListsRequireLoginAndRead(t *testing.T) {
	x := setup(t)
	x.expect(x.call(http.MethodGet, "/api/print/formats/Pessoa", nil, ""), http.StatusUnauthorized, "AuthenticationError")
	x.expect(x.call(http.MethodGet, "/api/letterheads", nil, ""), http.StatusUnauthorized, "AuthenticationError")
	zeAuth := "sid:" + x.sid("ze@x.com")
	// Pessoa lets everyone read their own records; Pedido is Gestor-only
	x.expect(x.call(http.MethodGet, "/api/print/formats/Pessoa", nil, zeAuth), http.StatusOK, "")
	x.expect(x.call(http.MethodGet, "/api/print/formats/Pedido", nil, zeAuth), http.StatusForbidden, "PermissionError")
	x.expect(x.call(http.MethodGet, "/api/print/formats/Pedido", nil, "sid:"+x.sid("ana@x.com")), http.StatusOK, "")
	x.expect(x.call(http.MethodGet, "/api/letterheads", nil, zeAuth), http.StatusOK, "")
}

// A failing query is an error, not an empty list of letterheads.
func TestPrintAPI_LetterheadsReportsQueryErrors(t *testing.T) {
	x := setup(t)
	if _, err := x.e.DB.Pool.Exec(x.ctx, `DROP TABLE tab_letter_head`); err != nil {
		t.Fatal(err)
	}
	r := x.call(http.MethodGet, "/api/letterheads", nil, "sid:"+x.sid("ana@x.com"))
	// Before, the handler returned [] and the request failed only at commit,
	// with a message that named nothing.
	if r.Status < 500 || !strings.Contains(r.Raw, "tab_letter_head") {
		t.Fatalf("expected the query error, got %d: %s", r.Status, r.Raw)
	}
}
