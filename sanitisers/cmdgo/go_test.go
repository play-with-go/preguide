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
			in:   "go version go1.17.5 linux/arm64\n",
			want: "go version go1.17.5 linux/amd64\n",
		},
		{
			san: sanitiseGoVersion{},
			in: `/home/myitcv/cues/cue: go1.18.5
        path    cuelang.org/go/cmd/cue
        mod     cuelang.org/go  (devel)
        dep     github.com/cockroachdb/apd/v2   v2.0.2  h1:weh8u7Cneje73dDh+2tEVLUvyBc89iwepWCD8b8034E=
        dep     github.com/emicklei/proto       v1.10.0 h1:pDGyFRVV5RvV+nkBK9iy3q67FBy9Xa7vwrOTE+g5aGw=
        dep     github.com/golang/glog  v0.0.0-20160126235308-23def4e6c14b      h1:VKtxabqXZkF25pY9ekfRL6a582T4P37/31XEstQ5p58=
        dep     github.com/google/uuid  v1.2.0  h1:qJYtXnJRWmpe7m/3XlyhrsLrEURqHRM2kxzoxXqyUDs=
        dep     github.com/mitchellh/go-wordwrap        v1.0.1  h1:TLuKupo69TCn6TQSyGxwI1EblZZEsQ0vMlAFQflz0v0=
        dep     github.com/mpvl/unique  v0.0.0-20150818121801-cbe035fff7de      h1:D5x39vF5KCwKQaw+OC9ZPiLVHXz3UFw2+psEX+gYcto=
        dep     github.com/pkg/errors   v0.8.1  h1:iURUrRGxPUNPdy5/HRSm+Yj6okJ6UtLINN0Q9M4+h3I=
        dep     github.com/protocolbuffers/txtpbfmt     v0.0.0-20220428173112-74888fd59c2b      h1:zd/2RNzIRkoGGMjE+YIsZ85CnDIz672JK2F3Zl4vux4=
        dep     github.com/spf13/cobra  v1.4.0  h1:y+wJpx64xcgO1V+RcnwW0LEHxTKRi2ZDPSBjWnrg88Q=
        dep     github.com/spf13/pflag  v1.0.5  h1:iy+VFUOCP1a+8yFto/drg2CJ5u0yRoB7fZw3DKv/JXA=
        dep     golang.org/x/mod        v0.6.0-dev.0.20220818022119-ed83ed61efb9        h1:VtCrPQXM5Wo9l7XN64SjBMczl48j8mkP+2e3OhYlz+0=
        dep     golang.org/x/net        v0.0.0-20220722155237-a158d28d115b      h1:PxfKdU9lEEDYjdIzOtC4qFWgkU2rGHdKlKowJSMN9h0=
        dep     golang.org/x/sys        v0.0.0-20220722155257-8c9f86f7a55f      h1:v4INt8xihDGvnrfjMDVXGxw9wrfxYyCjk0KbXjhR55s=
        dep     golang.org/x/text       v0.3.7  h1:olpwvP2KacW1ZWvsR7uQhoyTYvKAupfQrRGBFM352Gk=
        dep     golang.org/x/tools      v0.1.12 h1:VveCTK38A2rkS8ZqFY25HIDFscX5X9OoEhJd3quQmXU=
        dep     gopkg.in/yaml.v3        v3.0.1  h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
        build   -compiler=gc
        build   CGO_ENABLED=1
        build   CGO_CFLAGS=
        build   CGO_CPPFLAGS=
        build   CGO_CXXFLAGS=
        build   CGO_LDFLAGS=
        build   GOARCH=arm64
        build   GOOS=linux
        build   vcs=git
        build   vcs.revision=75189525aabd2bcf2d8738855b187adb4713a393
        build   vcs.time=2022-09-23T09:26:06Z
        build   vcs.modified=false
`,
			want: `/home/myitcv/cues/cue: go1.18.5
        path    cuelang.org/go/cmd/cue
        mod     cuelang.org/go  (devel)
        dep     github.com/cockroachdb/apd/v2   v2.0.2  h1:weh8u7Cneje73dDh+2tEVLUvyBc89iwepWCD8b8034E=
        dep     github.com/emicklei/proto       v1.10.0 h1:pDGyFRVV5RvV+nkBK9iy3q67FBy9Xa7vwrOTE+g5aGw=
        dep     github.com/golang/glog  v0.0.0-20160126235308-23def4e6c14b      h1:VKtxabqXZkF25pY9ekfRL6a582T4P37/31XEstQ5p58=
        dep     github.com/google/uuid  v1.2.0  h1:qJYtXnJRWmpe7m/3XlyhrsLrEURqHRM2kxzoxXqyUDs=
        dep     github.com/mitchellh/go-wordwrap        v1.0.1  h1:TLuKupo69TCn6TQSyGxwI1EblZZEsQ0vMlAFQflz0v0=
        dep     github.com/mpvl/unique  v0.0.0-20150818121801-cbe035fff7de      h1:D5x39vF5KCwKQaw+OC9ZPiLVHXz3UFw2+psEX+gYcto=
        dep     github.com/pkg/errors   v0.8.1  h1:iURUrRGxPUNPdy5/HRSm+Yj6okJ6UtLINN0Q9M4+h3I=
        dep     github.com/protocolbuffers/txtpbfmt     v0.0.0-20220428173112-74888fd59c2b      h1:zd/2RNzIRkoGGMjE+YIsZ85CnDIz672JK2F3Zl4vux4=
        dep     github.com/spf13/cobra  v1.4.0  h1:y+wJpx64xcgO1V+RcnwW0LEHxTKRi2ZDPSBjWnrg88Q=
        dep     github.com/spf13/pflag  v1.0.5  h1:iy+VFUOCP1a+8yFto/drg2CJ5u0yRoB7fZw3DKv/JXA=
        dep     golang.org/x/mod        v0.6.0-dev.0.20220818022119-ed83ed61efb9        h1:VtCrPQXM5Wo9l7XN64SjBMczl48j8mkP+2e3OhYlz+0=
        dep     golang.org/x/net        v0.0.0-20220722155237-a158d28d115b      h1:PxfKdU9lEEDYjdIzOtC4qFWgkU2rGHdKlKowJSMN9h0=
        dep     golang.org/x/sys        v0.0.0-20220722155257-8c9f86f7a55f      h1:v4INt8xihDGvnrfjMDVXGxw9wrfxYyCjk0KbXjhR55s=
        dep     golang.org/x/text       v0.3.7  h1:olpwvP2KacW1ZWvsR7uQhoyTYvKAupfQrRGBFM352Gk=
        dep     golang.org/x/tools      v0.1.12 h1:VveCTK38A2rkS8ZqFY25HIDFscX5X9OoEhJd3quQmXU=
        dep     gopkg.in/yaml.v3        v3.0.1  h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
`,
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
GOAMD64="v1"
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
