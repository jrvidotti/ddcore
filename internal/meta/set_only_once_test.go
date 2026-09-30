package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSetOnlyOnceValidation(t *testing.T) {
	var f Field
	if err := json.Unmarshal([]byte(`{"fieldname":"tenant","fieldtype":"Data","setOnlyOnce":true}`), &f); err != nil || !f.SetOnlyOnce {
		t.Fatalf("parsed %+v, %v", f, err)
	}
	if err := gridRegistry(&Field{Fieldname: "tenant", Fieldtype: "Data", SetOnlyOnce: true}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		f    *Field
		want string
	}{
		{&Field{Fieldname: "sec", Fieldtype: "Section Break", SetOnlyOnce: true}, `field "sec": setOnlyOnce is for a field with a stored value, not a Section Break`},
		{&Field{Fieldname: "s", Fieldtype: "Table", Options: "Course Student", SetOnlyOnce: true}, `field "s": setOnlyOnce is for a field with a stored value, not a Table`},
		{&Field{Fieldname: "total", Fieldtype: "Float", Computed: true, SetOnlyOnce: true}, `field "total": setOnlyOnce is for a field with a stored value, not a computed one`},
	} {
		err := gridRegistry(c.f).Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.f, c.want, err)
		}
	}
	// an app may impose it on another app's field (a tenant key, say)
	if !FieldProps["setOnlyOnce"] {
		t.Fatal("setOnlyOnce is not an extensible field property")
	}
}
