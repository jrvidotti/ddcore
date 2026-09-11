package acceptance

// The export over real HTTP: the headers a browser download depends on, the
// count a reconciliation checks, and the two refusals — no `export`
// permission, and a set too big to stream.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// getRaw does an authenticated GET and hands back the response with its body
// still unread — an export is a file, not the JSON envelope getJSON expects.
func getRaw(t *testing.T, srv *httptest.Server, tok, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "token "+tok)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func seedProjects(t *testing.T, e *engine.Engine, n int) {
	t.Helper()
	err := e.Run(context.Background(), "Administrator", func(c *engine.Ctx) error {
		for i := 0; i < n; i++ {
			d, err := c.NewDoc("Project", engine.Doc{
				"code":       fmt.Sprintf("P-%04d", i),
				"title":      fmt.Sprintf("Projeto %d", i),
				"assignee":   "Administrator",
				"start_date": "2026-01-01",
			})
			if err != nil {
				return err
			}
			d["milestones"] = []any{
				map[string]any{"title": "Kickoff", "due_date": "2026-02-01"},
				map[string]any{"title": "Entrega", "due_date": "2026-03-01"},
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExportCSVOverHTTP(t *testing.T) {
	e := setup(t, "exp")
	srv, tok := server(t, e)
	seedProjects(t, e, 30)

	res := getRaw(t, srv, tok, "/api/export/Project")
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type: %s", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".csv") {
		t.Errorf("Content-Disposition: %s", cd)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("falta nosniff")
	}
	count, err := strconv.Atoi(res.Header.Get("X-DDCore-Export-Count"))
	if err != nil || count != 30 {
		t.Fatalf("X-DDCore-Export-Count = %q (%v)", res.Header.Get("X-DDCore-Export-Count"), err)
	}

	body := readAll(t, res)
	if !strings.HasPrefix(body, "\ufeff") {
		t.Error("o CSV precisa do BOM para o Excel ler os acentos")
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatalf("CSV inválido: %v", err)
	}
	// The header the count promised: every row, plus the header line.
	if len(recs) != count+1 {
		t.Fatalf("esperava %d linhas + cabeçalho, veio %d", count, len(recs))
	}
	if !hasCol(recs[0], "code") || !hasCol(recs[0], "owner") || !hasCol(recs[0], "docstatus") {
		t.Errorf("faltam colunas de negócio ou de auditoria: %v", recs[0])
	}
}

// The desk's list export sends the filters it is showing; the file has to be
// the filtered set and the count has to agree with it.
func TestExportAppliesFiltersOverHTTP(t *testing.T) {
	e := setup(t, "expf")
	srv, tok := server(t, e)
	seedProjects(t, e, 20)

	// the % is a wildcard in the filter and an escape introducer in a URL:
	// it has to travel encoded, or the filter never arrives
	res := getRaw(t, srv, tok, `/api/export/Project?filters=`+url.QueryEscape(`[["code","like","P-000%"]]`))
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := res.Header.Get("X-DDCore-Export-Count"); got != "10" {
		t.Fatalf("contagem com filtro: %s", got)
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(readAll(t, res), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 11 {
		t.Fatalf("esperava 10 linhas + cabeçalho, veio %d", len(recs))
	}
}

// NDJSON is the reconcilable shape: children nested and a manifest to prove
// the stream was not cut short.
func TestExportNDJSONCarriesChildrenAndManifest(t *testing.T) {
	e := setup(t, "expn")
	srv, tok := server(t, e)
	seedProjects(t, e, 5)

	res := getRaw(t, srv, tok, "/api/export/Project?format=ndjson&children=1")
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Errorf("Content-Type: %s", ct)
	}
	lines := strings.Split(strings.TrimSpace(readAll(t, res)), "\n")
	if len(lines) != 6 {
		t.Fatalf("esperava 5 documentos + manifesto, veio %d", len(lines))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	ms, _ := first["milestones"].([]any)
	if len(ms) != 2 {
		t.Fatalf("os filhos não vieram aninhados: %v", first["milestones"])
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[5]), &last); err != nil {
		t.Fatal(err)
	}
	man, ok := last["_manifest"].(map[string]any)
	if !ok {
		t.Fatal("a última linha deveria ser o manifesto")
	}
	if man["rows"] != float64(5) {
		t.Errorf("manifesto: rows=%v", man["rows"])
	}
	if cr, _ := man["childRows"].(map[string]any); cr["Project Milestone"] != float64(10) {
		t.Errorf("manifesto: childRows=%v", man["childRows"])
	}
}

// CSV cannot hold the children; saying so beats dropping them in silence.
func TestExportRefusesChildrenInCSV(t *testing.T) {
	e := setup(t, "expc")
	srv, tok := server(t, e)

	res := getRaw(t, srv, tok, "/api/export/Project?format=csv&children=1")
	defer res.Body.Close()
	if res.StatusCode != 417 {
		t.Fatalf("esperava 417, veio %d", res.StatusCode)
	}
}

// The gate that did not exist before: `read` is not `export`.
func TestExportDeniedWithoutExportPermission(t *testing.T) {
	e := setup(t, "expp")
	srv, _ := server(t, e)
	seedProjects(t, e, 2)

	err := e.Run(context.Background(), "Administrator", func(c *engine.Ctx) error {
		d, err := c.NewDoc("User", engine.Doc{"email": "contrib@x.com", "full_name": "Contrib"})
		if err != nil {
			return err
		}
		d["roles"] = []any{map[string]any{"role": "Project Contributor"}}
		_, err = c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// the key is issued outside that transaction: it opens its own, and would
	// not see a user that has not been committed yet
	tok, err := e.CreateAPIKey(context.Background(), "contrib@x.com", "acc")
	if err != nil {
		t.Fatal(err)
	}

	res := getRaw(t, srv, tok, "/api/export/Project")
	defer res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("Project Contributor lê mas não exporta: esperava 403, veio %d", res.StatusCode)
	}
	// and the refusal is a JSON error, not a half-written file
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("uma recusa deveria vir como erro JSON, veio %s", ct)
	}
}

// Too big to stream is an answer with a way forward, not a stalled download.
func TestExportRefusesMoreThanTheCap(t *testing.T) {
	e := setup(t, "expl")
	e.Cfg.ExportMaxRows = 5
	srv, tok := server(t, e)
	seedProjects(t, e, 12)

	res := getRaw(t, srv, tok, "/api/export/Project")
	defer res.Body.Close()
	if res.StatusCode != 417 {
		t.Fatalf("esperava 417 acima do teto, veio %d", res.StatusCode)
	}
	body := readAll(t, res)
	if !strings.Contains(body, "ddcore export") {
		t.Errorf("a recusa deveria apontar a saída pela CLI: %s", body)
	}

	// An explicit limit is the caller accepting a sample, and is allowed.
	res2 := getRaw(t, srv, tok, "/api/export/Project?limit=3")
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Fatalf("com limit explícito esperava 200, veio %d", res2.StatusCode)
	}
	if got := res2.Header.Get("X-DDCore-Export-Count"); got != "3" {
		t.Errorf("contagem com limit: %s", got)
	}
}

func TestExportRejectsAnUnknownFormat(t *testing.T) {
	e := setup(t, "expu")
	srv, tok := server(t, e)

	res := getRaw(t, srv, tok, "/api/export/Project?format=xlsx")
	defer res.Body.Close()
	if res.StatusCode != 417 {
		t.Fatalf("esperava 417, veio %d", res.StatusCode)
	}
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasCol(header []string, name string) bool {
	for _, h := range header {
		if h == name {
			return true
		}
	}
	return false
}
