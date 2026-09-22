package engine

import (
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func seriesDT(g meta.IDGeneration, fields ...*meta.Field) *meta.DocType {
	return &meta.DocType{Name: "Pedido", IDGeneration: g, Fields: fields}
}

func TestSeriesCounterForReadsASeriesID(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "PED-.####"})
	key, n, ok := seriesCounterFor(d, Doc{"id": "PED-00042"})
	if !ok || key != "PED-" || n != 42 {
		t.Fatalf("key=%q n=%d ok=%v", key, n, ok)
	}
}

// The counter key is the rendered prefix, dates included, so a series with a
// year advances that year's counter and no other.
func TestSeriesCounterForKeepsTheDateInTheKey(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "CTR-.YYYY.-.#####"})
	key, n, ok := seriesCounterFor(d, Doc{"id": "CTR-2024-00007"})
	if !ok || key != "CTR-2024-" || n != 7 {
		t.Fatalf("key=%q n=%d ok=%v", key, n, ok)
	}
}

// A {field} segment renders from the document's own value.
func TestSeriesCounterForResolvesAFieldSegment(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: ".{loja}.-.###"}, &meta.Field{Fieldname: "loja", Fieldtype: "Data"})
	key, n, ok := seriesCounterFor(d, Doc{"id": "SP-015", "loja": "SP"})
	if !ok || key != "SP-" || n != 15 {
		t.Fatalf("key=%q n=%d ok=%v", key, n, ok)
	}
}

// id_series on the document overrides the DocType's series.
func TestSeriesCounterForHonoursAnIDSeriesOverride(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "PED-.####"}, &meta.Field{Fieldname: "id_series", Fieldtype: "Select"})
	key, n, ok := seriesCounterFor(d, Doc{"id": "ORC-0003", "id_series": "ORC-.####"})
	if !ok || key != "ORC-" || n != 3 {
		t.Fatalf("key=%q n=%d ok=%v", key, n, ok)
	}
}

// A format can put the counter in the middle; its key is the rendering with
// the counter taken out.
func TestSeriesCounterForReadsAFormatWithACounterInTheMiddle(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Format: "{imovel}-{###}-A"}, &meta.Field{Fieldname: "imovel", Fieldtype: "Data"})
	key, n, ok := seriesCounterFor(d, Doc{"id": "X9-021-A", "imovel": "X9"})
	if !ok || key != "X9--A" || n != 21 {
		t.Fatalf("key=%q n=%d ok=%v", key, n, ok)
	}
}

// An amendment keeps its original's id, so it never moves a counter.
func TestSeriesCounterForSkipsAnAmendment(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "PED-.####"})
	if _, _, ok := seriesCounterFor(d, Doc{"id": "PED-00042-1", "amended_from": "PED-00042"}); ok {
		t.Fatal("an amendment must not advance the counter")
	}
}

// An id that does not fit the pattern — a legacy key kept as it was — leaves
// the counter alone rather than guessing.
func TestSeriesCounterForIgnoresAnIDOutsideThePattern(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "PED-.####"})
	if _, _, ok := seriesCounterFor(d, Doc{"id": "LEGADO/77"}); ok {
		t.Fatal("an unrelated id must not advance the counter")
	}
	d2 := seriesDT(meta.IDGeneration{Field: "code"})
	if _, _, ok := seriesCounterFor(d2, Doc{"id": "ACME"}); ok {
		t.Fatal("a field-named DocType has no counter")
	}
}

// The counter is not capped by the declared width: the 100000th of a ####
// series is six digits long, and the next one has to follow it.
func TestSeriesCounterForReadsMoreDigitsThanDeclared(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "PED-.####"})
	_, n, ok := seriesCounterFor(d, Doc{"id": "PED-123456"})
	if !ok || n != 123456 {
		t.Fatalf("n=%d ok=%v", n, ok)
	}
}

// A date placeholder matches the digits that are actually in the id, not
// today's date: an import loads last year's documents.
func TestSeriesCounterForDoesNotAssumeToday(t *testing.T) {
	d := seriesDT(meta.IDGeneration{Series: "CTR-.YYYY.-.####"})
	last := time.Now().AddDate(-1, 0, 0).Format("2006")
	key, _, ok := seriesCounterFor(d, Doc{"id": "CTR-" + last + "-0001"})
	if !ok || key != "CTR-"+last+"-" {
		t.Fatalf("key=%q ok=%v", key, ok)
	}
}
