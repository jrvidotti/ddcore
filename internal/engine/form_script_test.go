package engine

import (
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestFormScript(t *testing.T) {
	files := []string{
		"doctypes/foo_bar_task/foo_bar_task.form.ts",
		"doctypes/task/task.form.ts",
		"doctypes/tagone_settings/tagone_settings.form.ts",
	}
	cases := []struct {
		name, source string
		files        []string
		want         string
	}{
		{"Task", "app.doctypes.task.task.doctype", files, "doctypes/task/task.form.ts"},
		// the snake of the name is tag_one_settings; the file follows the .doctype.ts
		{"TagOne Settings", "tagone.doctypes.tagone_settings.tagone_settings.doctype", files, "doctypes/tagone_settings/tagone_settings.form.ts"},
		// an extending app names its script after the DocType, and has no SourceFile of its own
		{"Task", "", files, "doctypes/task/task.form.ts"},
		{"Bar Task", "app.doctypes.bar_task.bar_task.doctype", files, ""},
		{"Task", "app.doctypes.task.task.doctype", []string{"doctypes/foo_bar_task/foo_bar_task.form.ts"}, ""},
		{"Task", "app.task.doctype", []string{"task.form.ts"}, "task.form.ts"},
	}
	for _, c := range cases {
		d := &meta.DocType{Name: c.name, SourceFile: c.source}
		if got := FormScript(c.files, d); got != c.want {
			t.Errorf("FormScript(%q, %q) = %q, want %q", c.name, c.source, got, c.want)
		}
	}
}
