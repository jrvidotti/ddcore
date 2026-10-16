package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// mergeApp is the shape of #129: a Unidade linked from a parent DocType, from
// the child rows of a submitted document, from a Dynamic Link and from a
// DocType that allows one row per Unidade.
func mergeApp() map[string]string {
	return map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor", "Leitor"] });`,
		"doctypes/unidade/unidade.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Unidade", idGeneration: { field: "nome" }, allowRename: true, trackChanges: true,
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "codigo", fieldtype: "Data", label: "Codigo", unique: true },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true },
    { role: "Leitor", read: true, write: true }] });`,
		"doctypes/unidade/unidade.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Unidade", {
  beforeMerge(doc) { if (doc.codigo === "recusa") ddcore.throw("merge refused by the app"); },
  afterMerge(doc) {
    const into = ddcore.getDoc("Unidade", doc.flags.mergeInto);
    if (!into.codigo) { into.codigo = doc.codigo; into.save(); }
  },
});`,
		"doctypes/funcionario/funcionario.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Funcionario", idGeneration: { field: "nome" },
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "unidade", fieldtype: "Link", label: "Unidade", options: "Unidade" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/loja/loja.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Loja", idGeneration: { field: "nome" },
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "unidade", fieldtype: "Link", label: "Unidade", options: "Unidade", unique: true },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/anexo/anexo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Anexo", idGeneration: { field: "nome" },
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "ref_type", fieldtype: "Data", label: "Tipo" },
    { fieldname: "ref_id", fieldtype: "Dynamic Link", label: "Documento", options: "ref_type" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/pagamento/pagamento.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pagamento", idGeneration: { field: "nome" }, submittable: true, allowRename: true,
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "itens", fieldtype: "Table", label: "Itens", options: "Pagamento Item" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, submit: true, cancel: true }] });`,
		"doctypes/pagamento_item/pagamento_item.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pagamento Item", isChild: true,
  fields: [{ fieldname: "unidade", fieldtype: "Link", label: "Unidade", options: "Unidade" }] });`,
	}
}

const (
	mergeUser   = "merge.user@x.com"
	mergeOther  = "merge.other@x.com"
	mergeLeitor = "merge.leitor@x.com"
)

func setupMerge(t *testing.T) *Engine {
	t.Helper()
	e := setupWith(t, mergeApp())
	runAs(t, e, "Admin", func(c *Ctx) error {
		for email, role := range map[string]string{mergeUser: "Gestor", mergeOther: "Gestor", mergeLeitor: "Leitor"} {
			if err := insertDoc(c, "User", Doc{"email": email, "full_name": email,
				"roles": []any{map[string]any{"role": role}}}); err != nil {
				return err
			}
		}
		for _, u := range []Doc{{"nome": "A", "codigo": "PW1"}, {"nome": "B"}} {
			if err := insertDoc(c, "Unidade", u); err != nil {
				return err
			}
		}
		return nil
	})
	return e
}

func mergeCount(t *testing.T, c *Ctx, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := c.Q().QueryRow(c.Ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// #129: every reference to the duplicate moves to the document that stays,
// and the duplicate is gone.
func TestMergeMovesEveryReference(t *testing.T) {
	e := setupMerge(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		// an edit, so A has a Version of its own to carry over
		a, err := c.GetDoc("Unidade", "A")
		if err != nil {
			return err
		}
		a["codigo"] = "PW2"
		if _, err := c.Save(a, SaveOpts{}); err != nil {
			return err
		}
		for _, f := range []Doc{{"nome": "F1", "unidade": "A"}, {"nome": "F2", "unidade": "B"}} {
			if err := insertDoc(c, "Funcionario", f); err != nil {
				return err
			}
		}
		if err := insertDoc(c, "Anexo", Doc{"nome": "X", "ref_type": "Unidade", "ref_id": "A"}); err != nil {
			return err
		}
		p, err := c.NewDoc("Pagamento", Doc{"nome": "P1", "itens": []any{map[string]any{"unidade": "A"}}})
		if err != nil {
			return err
		}
		if p, err = c.Insert(p, SaveOpts{}); err != nil {
			return err
		}
		if _, err := c.Submit(p); err != nil {
			return err
		}
		if err := insertDoc(c, "Comment", Doc{"comment_type": "Comment", "reference_doctype": "Unidade", "reference_id": "A", "content": "hi"}); err != nil {
			return err
		}
		if err := insertDoc(c, "ToDo", Doc{"allocated_to": mergeUser, "reference_type": "Unidade", "reference_id": "A", "description": "x"}); err != nil {
			return err
		}
		// mergeUser is shared both: B's share stays, A's goes with A
		for _, s := range []struct{ id, user string }{{"A", mergeUser}, {"B", mergeUser}, {"A", mergeOther}} {
			if _, err := c.ShareDoc("Unidade", s.id, s.user, ShareRights{Read: true}); err != nil {
				return err
			}
		}
		return insertDoc(c, "User Permission", Doc{"user": mergeOther, "allow": "Unidade", "for_value": "A"})
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		p, err := c.PreviewMerge("Unidade", "A", "B")
		if err != nil {
			return err
		}
		if len(p.Blockers) != 0 {
			t.Fatalf("blockers = %v", p.Blockers)
		}
		links := map[string]int64{}
		for _, l := range p.Links {
			links[l.Doctype+"."+l.Fieldname] = l.Count
		}
		for _, k := range []string{"Funcionario.unidade", "Pagamento Item.unidade", "Anexo.ref_id"} {
			if links[k] != 1 {
				t.Fatalf("preview links = %+v, want 1 for %s", p.Links, k)
			}
		}
		records := map[string]int64{}
		for _, r := range p.Records {
			records[r.Doctype] = r.Count
		}
		for _, k := range []string{"Comment", "ToDo", "Document Share", "User Permission"} {
			if records[k] == 0 {
				t.Fatalf("preview records = %+v, missing %s", p.Records, k)
			}
		}
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error { return c.Merge("Unidade", "A", "B") })
	runAs(t, e, "Admin", func(c *Ctx) error {
		if ok, _ := c.idExists("Unidade", "A"); ok {
			t.Fatal("A still exists")
		}
		checks := []struct {
			sql  string
			args []any
			want int64
		}{
			{`SELECT count(*) FROM tab_funcionario WHERE unidade = 'B'`, nil, 2},
			{`SELECT count(*) FROM tab_pagamento_item WHERE unidade = 'B'`, nil, 1},
			{`SELECT count(*) FROM tab_anexo WHERE ref_id = 'B'`, nil, 1},
			{`SELECT count(*) FROM tab_comment WHERE reference_doctype = 'Unidade' AND reference_id = 'B'`, nil, 1},
			{`SELECT count(*) FROM tab_to_do WHERE reference_type = 'Unidade' AND reference_id = 'B'`, nil, 1},
			{`SELECT count(*) FROM tab_document_share WHERE share_doctype = 'Unidade' AND share_id = 'B'`, nil, 2},
			{`SELECT count(*) FROM tab_document_share WHERE share_doctype = 'Unidade' AND share_id = 'A'`, nil, 0},
			{`SELECT count(*) FROM tab_user_permission WHERE allow = 'Unidade' AND for_value = 'B'`, nil, 1},
			// A's Version moved, next to the one afterMerge's save wrote; the
			// deletion Version names A
			{`SELECT count(*) FROM tab_version WHERE ref_doctype = 'Unidade' AND doc_id = 'B' AND NOT deleted`, nil, 2},
			{`SELECT count(*) FROM tab_version WHERE ref_doctype = 'Unidade' AND doc_id = 'A' AND deleted`, nil, 1},
			{`SELECT count(*) FROM tab_audit_event WHERE action = 'doc.merge' AND target_doctype = 'Unidade' AND target_id = 'B'`, nil, 1},
		}
		for _, ch := range checks {
			if got := mergeCount(t, c, ch.sql, ch.args...); got != ch.want {
				t.Errorf("%s = %d, want %d", ch.sql, got, ch.want)
			}
		}
		// afterMerge saw doc.flags.mergeInto and copied the code across
		b, err := c.GetDoc("Unidade", "B")
		if err != nil {
			return err
		}
		if b.Str("codigo") != "PW2" {
			t.Fatalf("afterMerge did not copy the code: %v", b)
		}
		return nil
	})
}

func TestMergeRefusals(t *testing.T) {
	e := setupMerge(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		wantStatus(t, c.Merge("Unidade", "A", "A"), 417)
		wantStatus(t, c.Merge("Unidade", "A", "missing"), 404)
		if err := insertDoc(c, "Funcionario", Doc{"nome": "F1"}); err != nil {
			return err
		}
		if err := insertDoc(c, "Funcionario", Doc{"nome": "F2"}); err != nil {
			return err
		}
		wantStatus(t, c.Merge("Funcionario", "F1", "F2"), 417) // no allowRename
		wantStatus(t, c.Merge("User", mergeUser, mergeOther), 417)
		// a submitted document is cancelled before it is merged
		for _, n := range []string{"P1", "P2"} {
			p, err := c.NewDoc("Pagamento", Doc{"nome": n})
			if err != nil {
				return err
			}
			if p, err = c.Insert(p, SaveOpts{}); err != nil {
				return err
			}
			if n == "P1" {
				if _, err := c.Submit(p); err != nil {
					return err
				}
			}
		}
		wantStatus(t, c.Merge("Pagamento", "P1", "P2"), 417)
		return nil
	})
	// one Loja per Unidade: two Unidades with a Loja each cannot be one
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, l := range []Doc{{"nome": "LA", "unidade": "A"}, {"nome": "LB", "unidade": "B"}} {
			if err := insertDoc(c, "Loja", l); err != nil {
				return err
			}
		}
		err := c.Merge("Unidade", "A", "B")
		wantStatus(t, err, 417)
		if !strings.Contains(err.Error(), "must be unique") {
			t.Fatalf("unique clash = %v", err)
		}
		p, err := c.PreviewMerge("Unidade", "A", "B")
		if err != nil {
			return err
		}
		if len(p.Blockers) != 1 {
			t.Fatalf("preview blockers = %v", p.Blockers)
		}
		return nil
	})
	// the app's beforeMerge refuses
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "Unidade", Doc{"nome": "R", "codigo": "recusa"}); err != nil {
			return err
		}
		err := c.Merge("Unidade", "R", "B")
		if err == nil || !strings.Contains(err.Error(), "merge refused by the app") {
			t.Fatalf("beforeMerge refusal = %v", err)
		}
		return nil
	})
	// write without delete is not enough
	runAs(t, e, mergeLeitor, func(c *Ctx) error {
		wantStatus(t, c.Merge("Unidade", "A", "B"), 403)
		_, err := c.PreviewMerge("Unidade", "A", "B")
		wantStatus(t, err, 403)
		return nil
	})
}

// A rename used to leave the User Permissions naming the document behind,
// and with them the user's scope.
func TestRenameCarriesUserPermissions(t *testing.T) {
	e := setupMerge(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		if err := insertDoc(c, "User Permission", Doc{"user": mergeOther, "allow": "Unidade", "for_value": "A"}); err != nil {
			return err
		}
		_, err := c.Rename("Unidade", "A", "C")
		return err
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		if n := mergeCount(t, c, `SELECT count(*) FROM tab_user_permission WHERE allow = 'Unidade' AND for_value = 'C'`); n != 1 {
			t.Fatalf("user permissions on the new name = %d", n)
		}
		return nil
	})
}

func TestMergeTree(t *testing.T) {
	e := setupTree(t)
	seedTree(t, e)
	runAs(t, e, "Admin", func(c *Ctx) error {
		// into its own descendant
		wantStatus(t, c.Merge("Test Territory", "Brazil", "South"), 417)
		// its children need a group to go to
		wantStatus(t, c.Merge("Test Territory", "Brazil", "Antarctica"), 417)
		return c.Merge("Test Territory", "Brazil", "World")
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		south, err := c.GetDoc("Test Territory", "South")
		if err != nil {
			return err
		}
		if got := db.Str(south["parent_test_territory"]); got != "World" {
			t.Fatalf("South's parent = %q", got)
		}
		return nil
	})
}

// The REST path and the SDK both reach Merge.
func TestMergeFromScript(t *testing.T) {
	e := setupMerge(t)
	out, _, err := e.Eval(context.Background(), `
ddcore.newDoc("Funcionario", { nome: "F1", unidade: "A" }).insert();
const r = ddcore.rename("Unidade", "A", "B", { merge: true });
[r, ddcore.db.getValue("Funcionario", "F1", "unidade"), ddcore.db.exists("Unidade", "A")]`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != `["B","B",null]` {
		t.Fatalf("script merge = %s", got)
	}
}

// A merge of a tenant-owned DocType runs inside the tenant: it moves that
// tenant's references and leaves another tenant's documents of the same ids
// alone.
func TestMergeInsideTenant(t *testing.T) {
	e := setupTenancy(t)
	for _, u := range []string{userA, userB} {
		runAs(t, e, u, func(c *Ctx) error {
			for _, n := range []string{"Ana", "Bia"} {
				if err := insertDoc(c, "Pessoa", Doc{"nome": n}); err != nil {
					return err
				}
			}
			return insertDoc(c, "Pedido", Doc{"cliente": "Ana"})
		})
	}
	runAs(t, e, userA, func(c *Ctx) error { return c.Merge("Pessoa", "Ana", "Bia") })
	runAs(t, e, userA, func(c *Ctx) error {
		if ok, _ := c.Exists("Pessoa", "Ana"); ok {
			t.Fatal("alfa's Ana survived the merge")
		}
		if n := mergeCount(t, c, `SELECT count(*) FROM tab_pedido WHERE cliente = 'Bia'`); n != 1 {
			t.Fatalf("alfa's Pedidos on Bia = %d", n)
		}
		return nil
	})
	runAs(t, e, userB, func(c *Ctx) error {
		if ok, _ := c.Exists("Pessoa", "Ana"); !ok {
			t.Fatal("beta's Ana went with alfa's merge")
		}
		if n := mergeCount(t, c, `SELECT count(*) FROM tab_pedido WHERE cliente = 'Ana'`); n != 1 {
			t.Fatalf("beta's Pedidos on Ana = %d", n)
		}
		return nil
	})
}

// Merging a Role moves its holders to the role that stays, and their cached
// roles say so at once.
func TestMergeRoleRefreshesCachedRoles(t *testing.T) {
	e := setupMerge(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, r := range []string{"QA A", "QA B"} {
			if err := insertDoc(c, "Role", Doc{"role_name": r}); err != nil {
				return err
			}
		}
		u, err := c.GetDoc("User", mergeUser)
		if err != nil {
			return err
		}
		u["roles"] = append(u.Children("roles"), map[string]any{"role": "QA A"})
		_, err = c.Save(u, SaveOpts{})
		return err
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		roles, err := c.RolesOf(mergeUser) // cached from here on
		if err != nil || !slices.Contains(roles, "QA A") {
			t.Fatalf("roles before = %v, %v", roles, err)
		}
		return c.Merge("Role", "QA A", "QA B")
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		roles, err := c.RolesOf(mergeUser)
		if err != nil || !slices.Contains(roles, "QA B") || slices.Contains(roles, "QA A") {
			t.Fatalf("roles after the merge = %v, %v", roles, err)
		}
		return nil
	})
}
