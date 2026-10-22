package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/print"
)

// pdfApp is a Receipt any Gestor may read but only System Manager may write,
// with a template of its own and mailApp's messages.
func pdfApp() map[string]string {
	files := mailApp()
	files["doctypes/receipt/receipt.doctype.ts"] = `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Receipt",
  fields: [
    { fieldname: "donor", fieldtype: "Data", label: "Donor" },
    { fieldname: "amount", fieldtype: "Currency", label: "Amount" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true },
    { role: "Gestor", read: true },
  ],
});`
	files["print/receipt_custom.print.ts"] = `import { definePrintTemplate, _ } from "@ddcore/sdk";
export default definePrintTemplate({
  name: "demo.receipt_custom",
  doctype: "Receipt",
  body: (doc, b, ctx) => [
    b.header(_("Official Receipt"), { subtitle: doc.id }),
    b.p(_("Received from {0}", [doc.donor])),
    b.keyValues([[_("Contribution"), ctx.formatCurrency(doc.amount)]]),
  ],
});`
	return files
}

// fakePDF makes the renderer copy the HTML through, so the "PDF" can be read.
func fakePDF(t *testing.T) {
	t.Setenv("DDCORE_GOTENBERG_URL", "")
	t.Setenv("DDCORE_PDF_COMMAND", "cp {in} {out}")
	print.ResetDefaultRenderer()
	t.Cleanup(print.ResetDefaultRenderer)
}

func setupPDF(t *testing.T) *Engine {
	t.Helper()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, pdfApp())}}, Test: true,
		Lang: "pt-BR", Currency: "BRL", DataDir: t.TempDir()})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": "ana@x.com", "full_name": "Ana"})
		if err != nil {
			return err
		}
		u["roles"] = []any{map[string]any{"role": "Gestor"}}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		_, err = c.Insert(Doc{"doctype": "Receipt", "id": "REC-1", "donor": "Carlos Silva", "amount": 250}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// evalJS runs code on the user's runtime, as server code would, and rolls
// back when rollback is set.
func evalJS(e *Engine, user, code string, rollback bool) (map[string]any, error) {
	var out map[string]any
	err := e.Run(context.Background(), user, func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		raw, err := rt.Eval(code)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		if rollback {
			return errRollback
		}
		return nil
	})
	if errors.Is(err, errRollback) {
		err = nil
	}
	return out, err
}

func pdfFiles(t *testing.T, e *Engine) []map[string]any {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT id FROM tab_file WHERE attached_to_doctype = 'Receipt'`)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// #136: server code renders a print template to a PDF File attached to the
// document, which is what sendMail's attach takes.
func TestPrintPDF_FromServerCode(t *testing.T) {
	fakePDF(t)
	e := setupPDF(t)

	f, err := evalJS(e, "Admin", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1", format: "demo.receipt_custom", language: "pt-BR" })`, false)
	if err != nil {
		t.Fatal(err)
	}
	if f["file_name"] != "demo.receipt_custom-REC-1.pdf" || f["content_type"] != "application/pdf" ||
		f["attached_to_doctype"] != "Receipt" || f["attached_to_id"] != "REC-1" {
		t.Fatalf("unexpected File: %v", f)
	}
	if url := db.Str(f["file_url"]); !strings.HasPrefix(url, "/private/files/") {
		t.Fatalf("the PDF must be private, got %s", url)
	}
	var content []byte
	err = e.Run(context.Background(), "Admin", func(c *Ctx) error {
		content, err = c.ReadAttachment(db.Str(f["file_url"]))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Official Receipt", "Carlos Silva", "250,00"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("the PDF lacks %q: %s", want, content)
		}
	}

	// the standard format is named after the DocType
	f, err = evalJS(e, "Admin", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1", letterHead: "none" })`, false)
	if err != nil {
		t.Fatal(err)
	}
	if f["file_name"] != "Receipt-REC-1.pdf" {
		t.Fatalf("standard print named %v", f["file_name"])
	}
}

// Attaching is writing: a reader cannot leave a File on the document unless
// the code says to ignore permissions.
func TestPrintPDF_AttachingNeedsWrite(t *testing.T) {
	fakePDF(t)
	e := setupPDF(t)

	_, err := evalJS(e, "ana@x.com", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1" })`, false)
	if ce := cerr.From(err); ce == nil || ce.Type != "PermissionError" {
		t.Fatalf("a reader attached a PDF: %v", err)
	}
	if n := len(pdfFiles(t, e)); n != 0 {
		t.Fatalf("a refused print left %d files", n)
	}
	if _, err := evalJS(e, "ana@x.com", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1", ignorePermissions: true })`, false); err != nil {
		t.Fatal(err)
	}
	if n := len(pdfFiles(t, e)); n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
}

func TestPrintPDF_RollbackLeavesNoFile(t *testing.T) {
	fakePDF(t)
	e := setupPDF(t)
	if _, err := evalJS(e, "Admin", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1" })`, true); err != nil {
		t.Fatal(err)
	}
	if n := len(pdfFiles(t, e)); n != 0 {
		t.Fatalf("a rolled back print left %d files", n)
	}
}

func TestPrintPDF_NoRendererIsUnavailable(t *testing.T) {
	t.Setenv("DDCORE_GOTENBERG_URL", "")
	t.Setenv("DDCORE_PDF_COMMAND", "")
	t.Setenv("PATH", "")
	if print.FindChrome() != "" {
		t.Skip("a Chrome is installed at a fixed path")
	}
	print.ResetDefaultRenderer()
	t.Cleanup(print.ResetDefaultRenderer)
	e := setupPDF(t)

	_, err := evalJS(e, "Admin", `ddcore.print.pdf({ doctype: "Receipt", id: "REC-1" })`, false)
	if ce := cerr.From(err); ce == nil || ce.Type != "UnavailableError" {
		t.Fatalf("expected UnavailableError, got %v", err)
	}
}

// The issue's own flow: print, then mail the File in the same call.
func TestPrintPDF_AttachedToAMessage(t *testing.T) {
	fakePDF(t)
	e := setupPDF(t)

	out, err := evalJS(e, "Admin", `(() => {
  const pdf = ddcore.print.pdf({ doctype: "Receipt", id: "REC-1", format: "demo.receipt_custom" });
  const sent = ddcore.sendMail({ template: "demo.aviso", to: "cliente@x.com", args: { pedido: "1", nome: "C", url: "https://x" },
    attach: [pdf.id], reference: { doctype: "Receipt", id: "REC-1" } });
  return { file: pdf.id, delivery: sent.delivery };
})()`, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT attachments FROM tab_email_delivery WHERE id = $1`, out["delivery"])
	if err != nil || len(rows) != 1 {
		t.Fatalf("delivery %v not found: %v", out["delivery"], err)
	}
	if got := asStrings(rows[0]["attachments"]); len(got) != 1 || got[0] != out["file"] {
		t.Fatalf("the message carries %v, want [%v]", got, out["file"])
	}
}
