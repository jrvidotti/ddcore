package meta

import (
	"encoding/json"
	"testing"
)

// noCopy is read by the desk's Duplicate alone: the meta only has to carry it
// to the desk, and let another app set it on a field it does not own.
func TestNoCopyReachesTheDesk(t *testing.T) {
	var f Field
	if err := json.Unmarshal([]byte(`{"fieldname":"ref","fieldtype":"Data","noCopy":true}`), &f); err != nil || !f.NoCopy {
		t.Fatalf("noCopy not parsed: %+v, %v", f, err)
	}
	out, _ := json.Marshal(f)
	var back map[string]any
	_ = json.Unmarshal(out, &back)
	if back["noCopy"] != true {
		t.Fatalf("noCopy not served: %s", out)
	}
	if !FieldProps["noCopy"] {
		t.Fatal("noCopy is not an extensible field property")
	}
}
