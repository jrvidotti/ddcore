package engine

import (
	"strings"
	"testing"
)

func TestRouteName(t *testing.T) {
	for in, want := range map[string]string{
		"Training Class": "TrainingClass",
		"Course":         "Course",
		" A  b\tC ":      "AbC",
	} {
		if got := routeName(in); got != want {
			t.Errorf("routeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRouteNameClash(t *testing.T) {
	if err := routeNameClash("workspace", []string{"Sales", "Human Resources", "Projects"}); err != nil {
		t.Fatalf("distinct names refused: %v", err)
	}
	// case is part of the name: only whitespace is dropped
	if err := routeNameClash("report", []string{"Open Tasks", "open tasks"}); err != nil {
		t.Fatalf("names differing by case refused: %v", err)
	}
	err := routeNameClash("workspace", []string{"HumanResources", "Sales", "Human Resources"})
	if err == nil {
		t.Fatal("two workspaces with one URL name were accepted")
	}
	for _, want := range []string{`workspace "HumanResources"`, `"Human Resources"`, "URL name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
