package engine

import (
	"testing"
	"time"

	"github.com/jrvidotti/cerne/internal/meta"
)

func TestCastValueDatetimeAceitaString(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	got, err := castValue(f, "2026-09-10 17:35:20")
	if err != nil {
		t.Fatalf("castValue() erro = %v", err)
	}
	if _, ok := got.(time.Time); !ok {
		t.Fatalf("castValue() = %#v, queria time.Time", got)
	}
}

// castAll grava o valor convertido de volta no documento: coagir de novo o
// mesmo campo não pode falhar (era o caso quando o app gravava duas vezes).
func TestCastValueDatetimeEhIdempotente(t *testing.T) {
	f := &meta.Field{Fieldname: "concluida_em", Fieldtype: "Datetime", Label: "Concluída em"}
	primeiro, err := castValue(f, "2026-09-10 17:35:20")
	if err != nil {
		t.Fatalf("castValue() erro = %v", err)
	}
	segundo, err := castValue(f, primeiro)
	if err != nil {
		t.Fatalf("segunda castValue() erro = %v", err)
	}
	if !segundo.(time.Time).Equal(primeiro.(time.Time)) {
		t.Fatalf("segunda castValue() = %v, queria %v", segundo, primeiro)
	}
}
