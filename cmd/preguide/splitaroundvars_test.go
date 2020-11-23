package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSplitAroundVars(t *testing.T) {
	vs := []struct {
		in  string
		out []string
	}{
		{"", []string{""}},
		{"test", []string{"test"}},
		{"$test", []string{"", "test", ""}},
		{"${test}", []string{"", "test", ""}},
		{"${}", []string{"${}"}},
		{"$ test", []string{"$ test"}},
		{"$ test $var", []string{"$ test ", "var", ""}},
		{"${ test}", []string{"", " test", ""}},
		{"${regex_float}hello$ go test", []string{"", "regex_float", "hello$ go test"}},
		{"term1$ go test${regex_float}s", []string{"term1$ go test", "regex_float", "s"}},
	}
	for _, v := range vs {
		got := splitAroundVars(v.in)
		if !cmp.Equal(got, v.out) {
			t.Fatalf("splitAroundVars(%q): %v", v.in, cmp.Diff(got, v.out))
		}
	}
}

func splitAroundVars(s string) (parts []string) {
	// ${} is all ASCII, so bytes are fine for this operation.
	i := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '$' && j+1 < len(s) {
			name, w := getShellName(s[j+1:])
			if name != "" {
				parts = append(parts, string(s[i:j]), name)
				i = j + w + 1 // the +1 is the dollar
			}
			j += w // the +1 for the dollar is added in the for loop
		}
	}
	if len(parts) == 0 {
		return []string{s}
	}
	parts = append(parts, s[i:])
	return parts
}

// getShellName returns the name that begins the string and the number of bytes
// consumed to extract it. If the name is enclosed in {}, it's part of a ${}
// expansion and two more bytes are needed than the length of the name.
func getShellName(s string) (string, int) {
	switch {
	case s[0] == '{':
		if len(s) > 2 && isShellSpecialVar(s[1]) && s[2] == '}' {
			return s[1:2], 3
		}
		// Scan to closing brace
		for i := 1; i < len(s); i++ {
			if s[i] == '}' {
				if i == 1 {
					return "", 2 // Bad syntax; eat "${}"
				}
				return s[1:i], i + 1
			}
		}
		return "", 1 // Bad syntax; eat "${"
	case isShellSpecialVar(s[0]):
		return s[0:1], 1
	}
	// Scan alphanumerics.
	var i int
	for i = 0; i < len(s) && isAlphaNum(s[i]); i++ {
	}
	return s[:i], i
}

func isShellSpecialVar(c uint8) bool {
	switch c {
	case '*', '#', '$', '@', '!', '?', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}
	return false
}

func isAlphaNum(c uint8) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
