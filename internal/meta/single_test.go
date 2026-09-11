package meta

import "testing"

func TestSingleIncompatibleMetadata(t *testing.T) {
	for _, change := range []func(*DocType){
		func(d *DocType) { d.IsChild = true },
		func(d *DocType) { d.Submittable = true },
		func(d *DocType) { d.AllowRename = true },
		func(d *DocType) { d.Naming = Naming{Hash: true} },
	} {
		d := &DocType{Name: "Settings", IsSingle: true}
		change(d)
		r := NewRegistry()
		r.Add(d)
		if err := r.Validate(); err == nil {
			t.Fatalf("incompatible metadata accepted: %+v", d)
		}
	}
}
