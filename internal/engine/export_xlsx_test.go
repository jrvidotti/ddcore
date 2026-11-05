package engine

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

func exportXLSX(t *testing.T, e *Engine, a ExportArgs, loc *time.Location) map[string]*tabular.Sheet {
	t.Helper()
	out := map[string]*bytes.Buffer{}
	err := e.Run(context.Background(), "exp@x.com", func(c *Ctx) error {
		sink := &XLSXSink{
			Open: func(table string) (io.Writer, error) {
				out[table] = &bytes.Buffer{}
				return out[table], nil
			},
			Lookup:        func(n string) (*meta.DocType, error) { return c.St.DocType(n) },
			Loc:           loc,
			MoneyDecimals: 2,
		}
		_, err := c.Export(a, sink)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sheets := map[string]*tabular.Sheet{}
	for table, b := range out {
		s, err := tabular.Read(b.Bytes(), tabular.Options{})
		if err != nil {
			t.Fatalf("%s is not a workbook: %v", table, err)
		}
		sheets[table] = s
	}
	return sheets
}

func cellsByHeader(s *tabular.Sheet, row int) map[string]tabular.Cell {
	m := map[string]tabular.Cell{}
	for i, h := range s.Headers {
		if i < len(s.Rows[row].Cells) {
			m[h] = s.Rows[row].Cells[i]
		}
	}
	return m
}

// The XLSX carries the CSV's columns with their types: a Currency is a
// number, a Check a boolean, a Datetime a day number on the site's clock.
func TestXLSXSinkTypesCells(t *testing.T) {
	e := setupExport(t)
	var creation, modified string
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": "Endereço", "valor": 10.5, "ativo": false})
		if _, err := c.Insert(d, SaveOpts{}); err != nil {
			return err
		}
		saved, err := c.GetDoc("Nota", "Endereço")
		if err == nil {
			creation, _ = saved["creation"].(string)
			modified, _ = saved["modified"].(string)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	loc := time.FixedZone("BRT", -3*3600)
	sheets := exportXLSX(t, e, ExportArgs{Doctype: "Nota", Fields: []string{"id", "titulo", "valor", "ativo", "creation", "docstatus", "modified"}}, loc)
	s := sheets["Nota"]
	if s == nil || len(sheets) != 1 {
		t.Fatalf("expected one workbook, got %d", len(sheets))
	}
	if len(s.Headers) != 7 || s.Headers[0] != "id" || s.Headers[2] != "valor" {
		t.Fatalf("the CSV's header, in the order asked: %v", s.Headers)
	}
	if len(s.Rows) != 1 {
		t.Fatalf("rows: %d", len(s.Rows))
	}
	c := cellsByHeader(s, 0)
	if c["titulo"].Text != "Endereço" {
		t.Errorf("titulo: %+v", c["titulo"])
	}
	if c["valor"].Num == nil || *c["valor"].Num != 10.5 {
		t.Errorf("a Currency is a number: %+v", c["valor"])
	}
	if c["ativo"].Bool == nil || *c["ativo"].Bool {
		t.Errorf("a Check is a boolean: %+v", c["ativo"])
	}
	if c["docstatus"].Num == nil || *c["docstatus"].Num != 0 {
		t.Errorf("docstatus is a number: %+v", c["docstatus"])
	}
	// modified stays the exact text the CSV holds: Data Import compares it
	if c["modified"].Num != nil || c["modified"].Text != modified {
		t.Errorf("modified: %+v, want the text %q", c["modified"], modified)
	}
	at, err := time.Parse(time.RFC3339Nano, creation)
	if err != nil {
		t.Fatalf("creation %q: %v", creation, err)
	}
	if c["creation"].Num == nil {
		t.Fatalf("a Datetime is a day number: %+v", c["creation"])
	}
	// the wall clock in the site's zone, to the millisecond the serial keeps
	got := tabular.ExcelSerialToTime(*c["creation"].Num, false)
	want := at.In(loc)
	wall := time.Date(want.Year(), want.Month(), want.Day(), want.Hour(), want.Minute(), want.Second(), want.Nanosecond(), time.UTC)
	if d := got.Sub(wall); d > time.Millisecond || d < -time.Millisecond {
		t.Errorf("creation reads %v, want the site's wall clock %v", got, wall)
	}
}

// An export that matches nothing is still a workbook — the header and no row.
func TestXLSXSinkWritesAnEmptyWorkbook(t *testing.T) {
	e := setupExport(t)
	sheets := exportXLSX(t, e, ExportArgs{Doctype: "Nota"}, nil)
	s := sheets["Nota"]
	if s == nil || len(s.Rows) != 0 || len(s.Headers) == 0 || s.Headers[0] != "id" {
		t.Fatalf("expected a header-only workbook, got %+v", s)
	}
}

// Children go to a workbook of their own, keyed back to the parent, as in CSV.
func TestXLSXSinkSplitsChildTables(t *testing.T) {
	e := setupExport(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": "Com itens"})
		d["itens"] = []any{map[string]any{"descricao": "a", "qtd": 1}, map[string]any{"descricao": "b", "qtd": 2}}
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sheets := exportXLSX(t, e, ExportArgs{Doctype: "Nota", Children: true}, nil)
	child := sheets["Nota.itens"]
	if sheets["Nota"] == nil || child == nil {
		t.Fatalf("expected one workbook per table, got %d", len(sheets))
	}
	if len(child.Rows) != 2 || !containsFold(child.Headers, "parent") {
		t.Fatalf("child workbook: %v, %d rows", child.Headers, len(child.Rows))
	}
	c := cellsByHeader(child, 1)
	if c["qtd"].Num == nil || *c["qtd"].Num != 2 || c["idx"].Num == nil {
		t.Errorf("an Int and idx are numbers: %+v", c)
	}
}

func TestXLSXCellTypesEachFieldtype(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	serial := func(v any) float64 {
		d, ok := v.(tabular.XLSXDate)
		if !ok {
			t.Fatalf("not a date: %#v", v)
		}
		return d.Serial
	}
	if got := serial(xlsxCell("2026-03-01", "Date", loc)); got != 46082 {
		t.Errorf("Date: %v", got)
	}
	// 12:00 UTC is 09:00 on the site's clock
	if got := serial(xlsxCell("2026-03-01T12:00:00Z", "Datetime", loc)); got != 46082.375 {
		t.Errorf("Datetime: %v", got)
	}
	if got := serial(xlsxCell("18:00:00", "Time", loc)); got != 0.75 {
		t.Errorf("Time: %v", got)
	}
	if got := xlsxCell(int64(3), "Int", loc); got != 3.0 {
		t.Errorf("Int: %#v", got)
	}
	if got := xlsxCell(12.5, "Currency", loc); got != tabular.XLSXMoney(12.5) {
		t.Errorf("Currency: %#v", got)
	}
	if got := xlsxCell("Open", "Select", loc); got != "Open" {
		t.Errorf("a Select keeps its value: %#v", got)
	}
	if got := xlsxCell(map[string]any{"a": 1}, "JSON", loc); got != `{"a":1}` {
		t.Errorf("JSON is its compact text: %#v", got)
	}
	if got := xlsxCell("not a date", "Date", loc); got != "not a date" {
		t.Errorf("a value that does not parse stays text: %#v", got)
	}
	if got := xlsxCell(true, "Data", loc); got != "true" {
		t.Errorf("a boolean in a text column reads as the CSV: %#v", got)
	}
}
