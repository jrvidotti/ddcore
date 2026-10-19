package mcp

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// Every fieldtype the framework accepts is named in the scaffold_doctype
// description and in the SDK's Fieldtype union.
func TestFieldtypesAreDocumented(t *testing.T) {
	sdk, err := os.ReadFile("../../packages/sdk/src/types.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, ft := range meta.ValidFieldTypes {
		if !regexp.MustCompile(`(: |, )` + regexp.QuoteMeta(ft) + `(,| \(|\.)`).MatchString(scaffoldDescription) {
			t.Errorf("scaffold_doctype description does not mention %q", ft)
		}
		if !strings.Contains(string(sdk), `"`+ft+`"`) {
			t.Errorf("packages/sdk/src/types.ts does not list %q", ft)
		}
	}
}
