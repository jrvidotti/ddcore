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
