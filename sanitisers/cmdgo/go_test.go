package cmdgo

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/mod/semver"
)

func TestSanitiseGoTest(t *testing.T) {
	testCases := []struct {
		in   string
		want string
	}{{
		in: `=== RUN   TestGood
=== RUN   TestGood/something
ok
--- PASS: TestGood (0.00s)
    --- PASS: TestGood/something (0.00s)
=== RUN   TestBad
=== RUN   TestBad/something
    TestBad/something: main_test.go:16: bad
--- FAIL: TestBad (0.00s)
    --- FAIL: TestBad/something (0.00s)
FAIL
FAIL	mod.go	0.002s
=== RUN   TestP
    TestP: p_test.go:6: =========
--- FAIL: TestP (0.00s)
FAIL
FAIL	 mod.go/p 	0.002s
FAIL
`,
		want: `=== RUN   TestGood
=== RUN   TestGood/something
ok
--- PASS: TestGood (0.042s)
    --- PASS: TestGood/something (0.042s)
=== RUN   TestBad
=== RUN   TestBad/something
    TestBad/something: main_test.go:16: bad
--- FAIL: TestBad (0.042s)
    --- FAIL: TestBad/something (0.042s)
FAIL
FAIL	mod.go	0.042s
=== RUN   TestP
    TestP: p_test.go:6: =========
--- FAIL: TestP (0.042s)
FAIL
FAIL	 mod.go/p 	0.042s
FAIL
`,
	}}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestSanitiseGoTest_%v", i), func(t *testing.T) {
			got := sanitiseGoTest(nil, tc.in)
			if got != tc.want {
				t.Fatalf("failed to get sanitised output: %v", cmp.Diff(got, tc.want))
			}
		})
	}
}

func TestPseudoVersion(t *testing.T) {
	testCases := []struct {
		in   string
		want bool
	}{
		{"v0.0.0-20200901194510-cc2d21bd1e55", true},
		{"v2.3.0-pre.0.20060102150405-hash+incompatible", true},
		{"v1.0.1-0.20060102150405-hash+metadata", true},
		{"v1.4.3", false},
		{"v1.4.3-other", false},
		{"v1.4.3+something", false},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestPseudoVersion_%v", i), func(t *testing.T) {
			got := pseudoVersion.MatchString(tc.in)
			if !semver.IsValid(tc.in) {
				t.Errorf("semver.IsValid(%q) == false, want true", tc.in)
			}
			if got && !tc.want {
				t.Errorf("got a match where we did not expect one")
			} else if !got && tc.want {
				t.Errorf("failed to find a match where we expected one ")
			}
		})
	}
}

func TestSanitiseGoGet(t *testing.T) {
	testCases := []struct {
		vars []string
		in   string
		want string
	}{{
		vars: []string{"{{.REPO1}}"},
		in: `go: downloading play-with-go.dev/userguides/{{.REPO1}} v2.0.0-20200901194510-cc2d21bd1e55+something
go: play-with-go.dev/userguides/{{.REPO1}} upgrade => v0.0.0-20200901194510-cc2d21bd1e55
`,
		want: `go: downloading play-with-go.dev/userguides/{{.REPO1}} v2.0.0-20060102150405-abcde12345+something
go: play-with-go.dev/userguides/{{.REPO1}} upgrade => v0.0.0-20060102150405-abcde12345
`,
	}}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestSanitiseGoGet_%v", i), func(t *testing.T) {
			got := sanitiseGoGet(tc.vars, tc.in)
			if got != tc.want {
				t.Fatalf("failed to get sanitised output: %v", cmp.Diff(got, tc.want))
			}
		})
	}

}
