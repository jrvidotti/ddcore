package engine

import (
	"fmt"
	"testing"

	"github.com/jrvidotti/ddcore/internal/storage"
)

// ddcore.files.thumbUrl builds the ?w= url of #154 from the widths the server
// serves, and leaves every other url alone.
func TestFilesThumbUrl(t *testing.T) {
	e := setup(t)
	out, err := evalJS(e, "Admin", `({
	  small: ddcore.files.thumbUrl("/files/a.png", 40),
	  exact: ddcore.files.thumbUrl("/files/a.JPG", 128),
	  between: ddcore.files.thumbUrl("/files/a.webp", 129),
	  huge: ddcore.files.thumbUrl("/files/a.gif", 4000),
	  private: ddcore.files.thumbUrl("/private/files/a.png", 64),
	  pdf: ddcore.files.thumbUrl("/files/a.pdf", 64),
	  external: ddcore.files.thumbUrl("https://x.com/a.png", 64),
	  empty: ddcore.files.thumbUrl(null, 64),
	  widths: [1, 65, 129, 257].map((w) => ddcore.files.thumbUrl("/files/a.png", w).split("=")[1]),
	})`, true)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"small": "/files/a.png?w=64", "exact": "/files/a.JPG?w=128", "between": "/files/a.webp?w=256",
		"huge": "/files/a.gif?w=512", "private": "/private/files/a.png", "pdf": "/files/a.pdf",
		"external": "https://x.com/a.png", "empty": "",
	} {
		if got := fmt.Sprint(out[k]); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// the prelude's list is the server's
	if got, want := fmt.Sprint(out["widths"]), fmt.Sprint(storage.ThumbWidths); got != want {
		t.Errorf("prelude widths %s, storage.ThumbWidths %s", got, want)
	}
	if _, err := evalJS(e, "Admin", `({ v: ddcore.files.thumbUrl("/files/a.png", 0) })`, true); err == nil {
		t.Error("a width of 0 was accepted")
	}
}
