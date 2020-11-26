package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/play-with-go/preguide/internal/types"
)

func TestComment(t *testing.T) {
	vs := []struct {
		lang string
		in   string
		out  string
	}{
		{"go", "this is a test", "// this is a test"},
		{"go", "this is \na test", "// this is \n// a test"},
		{"sh", "this is", "# this is"},
		{"md", "this is", "&lt;!-- this is --&gt;"},
	}
	for _, v := range vs {
		got := comment(types.ModeJekyll, v.in, v.lang)
		if got != v.out {
			t.Errorf("comment(%q, %q); [-got, +want]\n%v", v.in, v.lang, cmp.Diff(got, v.out))
		}
	}
}
