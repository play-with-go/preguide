// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package cmdgo

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

func TestSanitiseOutput(t *testing.T) {
	testCases := []struct {
		san  sanitisers.Sanitiser
		in   string
		want string
	}{
		{
			san: sanitiseGoGet{},
			in: `go get example.com/private: module example.com/private: reading https://proxy.golang.org/example.com/private/@v/list: 410 Gone
	server response:
	not found: module example.com/private: git ls-remote -q origin in /tmp/gopath/pkg/mod/cache/vcs/21d3ff96908bdf3e8553891caf86061ba84ba5b6ea4700a65e79b3d3e38384e9: exit status 128:
		fatal: could not read Username for 'https://gopher.live': terminal prompts disabled
	Confirm the import path was entered correctly.
	If this is a private repository, see https://golang.org/doc/faq#git_https for additional information.
`,
			want: `go get example.com/private: module example.com/private: reading https://proxy.golang.org/example.com/private/@v/list: 410 Gone
	server response:
	not found: module example.com/private: git ls-remote -q origin in /tmp/gopath/pkg/mod/cache/vcs/0123456789abcdef: exit status 128:
		fatal: could not read Username for 'https://gopher.live': terminal prompts disabled
	Confirm the import path was entered correctly.
	If this is a private repository, see https://golang.org/doc/faq#git_https for additional information.
`,
		},
		{
			san: sanitiseGoTest{bench: true},
			in: `goos: linux
goarch: arm64
pkg: mod.com
cpu: Intel(R) Core(TM) i7-4960HQ CPU @ 2.60GHz
BenchmarkFib10-8   	 3357487	       333.7 ns/op
PASS
ok  	mod.com	1.489s
`,
			want: `goos: linux
goarch: amd64
pkg: mod.com
BenchmarkFib10-8   	 3357487	       333.7 ns/op
PASS
ok  	mod.com	1.489s
`,
		},
		{
			san:  sanitiseGoVersion{},
			in:   `go version go1.17.5 linux/arm64`,
			want: `go version go1.17.5 linux/amd64`,
		},
		{
			san: sanitiseGoEnv{},
			in: `GO111MODULE=""
GOARCH="arm64"
GOBIN=""
GOCACHE="/home/myitcv/.cache/go-build"
GOENV="/home/myitcv/.config/go/env"
GOEXE=""
GOFLAGS=""
GOHOSTARCH="arm64"
GOHOSTOS="linux"
GOINSECURE=""
GOMODCACHE="/home/myitcv/gostuff/pkg/mod"
GONOPROXY=""
GONOSUMDB=""
GOOS="linux"
GOPATH="/home/myitcv/gostuff"
GOPRIVATE=""
GOPROXY="https://proxy.golang.org,direct"
GOROOT="/home/myitcv/gos"
GOSUMDB="sum.golang.org"
GOTMPDIR=""
GOTOOLDIR="/home/myitcv/gos/pkg/tool/linux_arm64"
GCCGO="gccgo"
AR="ar"
CC="gcc"
CXX="g++"
CGO_ENABLED="1"
GOMOD="/home/myitcv/dev/learn.go.dev/preguide/go.mod"
CGO_CFLAGS="-g -O2"
CGO_CPPFLAGS=""
CGO_CXXFLAGS="-g -O2"
CGO_FFLAGS="-g -O2"
CGO_LDFLAGS="-g -O2"
PKG_CONFIG="pkg-config"
GOGCCFLAGS="-fPIC -m64 -pthread -fmessage-length=0 -fdebug-prefix-map=/tmp/go-build151771040=/tmp/go-build -gno-record-gcc-switches"
`,
			want: `GO111MODULE=""
GOARCH="amd64"
GOBIN=""
GOCACHE="/home/myitcv/.cache/go-build"
GOENV="/home/myitcv/.config/go/env"
GOEXE=""
GOFLAGS=""
GOHOSTARCH="amd64"
GOHOSTOS="linux"
GOINSECURE=""
GOMODCACHE="/home/myitcv/gostuff/pkg/mod"
GONOPROXY=""
GONOSUMDB=""
GOOS="linux"
GOPATH="/home/myitcv/gostuff"
GOPRIVATE=""
GOPROXY="https://proxy.golang.org,direct"
GOROOT="/home/myitcv/gos"
GOSUMDB="sum.golang.org"
GOTMPDIR=""
GOTOOLDIR="/home/myitcv/gos/pkg/tool/linux_amd64"
GCCGO="gccgo"
AR="ar"
CC="gcc"
CXX="g++"
CGO_ENABLED="1"
GOMOD="/home/myitcv/dev/learn.go.dev/preguide/go.mod"
CGO_CFLAGS="-g -O2"
CGO_CPPFLAGS=""
CGO_CXXFLAGS="-g -O2"
CGO_FFLAGS="-g -O2"
CGO_LDFLAGS="-g -O2"
PKG_CONFIG="pkg-config"
GOGCCFLAGS="fake_gcc_flags"
`,
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestSanitiseOuputGoTest_%v", i), func(t *testing.T) {
			san := tc.san
			got := san.Output(nil, tc.in)
			if got != tc.want {
				t.Fatalf("failed to get sanitised output: %v", cmp.Diff(got, tc.want))
			}
		})
	}
}

func TestSanitiseComparisonOutput(t *testing.T) {
	testCases := []struct {
		san  sanitisers.Sanitiser
		in   string
		want string
	}{
		{
			san: sanitiseGoTest{},
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
--- PASS: TestGood (N.NNs)
    --- PASS: TestGood/something (N.NNs)
=== RUN   TestBad
=== RUN   TestBad/something
    TestBad/something: main_test.go:16: bad
--- FAIL: TestBad (N.NNs)
    --- FAIL: TestBad/something (N.NNs)
FAIL
FAIL	mod.go	N.NNs
=== RUN   TestP
    TestP: p_test.go:6: =========
--- FAIL: TestP (N.NNs)
FAIL
FAIL	 mod.go/p 	N.NNs
FAIL
`,
		},
		{
			san: sanitiseGoTest{bench: true},
			in: `goos: linux
goarch: arm64
pkg: mod.com
cpu: Intel(R) Core(TM) i7-4960HQ CPU @ 2.60GHz
BenchmarkFib10-8   	 3357487	       333.7 ns/op
PASS
ok  	mod.com	1.489s
`,
			want: `goos: linux
goarch: arm64
pkg: mod.com
cpu: Intel(R) Core(TM) i7-4960HQ CPU @ 2.60GHz
BenchmarkFib10-8 NN N ns/op
PASS
ok  	mod.com	N.NNs
`,
		},
		{
			san: sanitiseGoGet{},
			in: `go: downloading golang.org/x/tools v0.0.0-20201105220310-78b158585360
go: found golang.org/x/tools/cmd/stringer in golang.org/x/tools v0.0.0-20201105220310-78b158585360
go: downloading golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
go: downloading golang.org/x/mod v0.3.0
`,
			want: `
go: downloading golang.org/x/mod v0.3.0
go: downloading golang.org/x/tools v0.0.0-20201105220310-78b158585360
go: downloading golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
go: found golang.org/x/tools/cmd/stringer in golang.org/x/tools v0.0.0-20201105220310-78b158585360`,
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("TestSanitiseComparisonOutputGoGet_%v", i), func(t *testing.T) {
			got := tc.san.ComparisonOutput(nil, tc.in)
			if got != tc.want {
				t.Fatalf("failed to get sanitised output: %v", cmp.Diff(got, tc.want))
			}
		})
	}
}

func TestCmdGoStmtSanitiser(t *testing.T) {
	// Deliberately make the input multiline
	input := "(\n cd $(mktemp -d);\nGO111MODULE=on go get honnef.co/go/tools/cmd/staticcheck@v0.0.1-2020.1.6)"
	r := strings.NewReader(input)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		t.Fatalf("failed to parse control input: %v", err)
	}
	stmt := f.Stmts[0]
	s := sanitisers.NewS()
	var want sanitiseGoGet
	if got := CmdGoStmtSanitiser(s, stmt); want != got {
		t.Fatalf("CmdGoStmtSanitiser(%q) == %T; wanted %T", input, got, want)
	}
}
