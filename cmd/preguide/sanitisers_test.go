package main

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
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
