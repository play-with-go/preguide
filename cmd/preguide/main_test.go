// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"cuelang.org/go/cue/load"
	"github.com/play-with-go/preguide/internal/textutil"
	"github.com/play-with-go/preguide/internal/util"
	"github.com/rogpeppe/go-internal/gotooltest"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"cmd-cmpregex": cmdCmpRegex,
		"preguide": func() int {
			self := os.Getenv("PREGUIDE_SELF_BUILD")
			if self == "" {
				fmt.Fprintln(os.Stderr, "PREGUIDE_SELF_BUILD env var not set")
				return 1
			}
			runSelf(self)
			return 0
		},
	}))
}

func TestScripts(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required to run preguide tests")
	}

	selfBuild := buildSelf(t)

	var m sync.Mutex

	p := testscript.Params{
		UpdateScripts: os.Getenv("PREGUIDE_UPDATE") == "1",
		Dir:           filepath.Join("testdata"),
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"envsubst":            envsubst,
			"startserver":         startserver,
			"createdockernetwork": createdockernetwork,
			"cmpregex":            cmpregex,
		},
		Setup: func(env *testscript.Env) (err error) {
			defer util.HandleKnown(&err)

			env.Vars = append(env.Vars,
				"PREGUIDE_IMAGE_OVERRIDE="+os.Getenv("PREGUIDE_IMAGE_OVERRIDE"),
				"PREGUIDE_PULL_IMAGE=missing",
				"PREGUIDE_SELF_BUILD="+selfBuild,
				"PREGUIDE_NO_DEVEL_HASH=true",
			)

			// Despite the fact that preguide embeds the definitions it needs,
			// it's more convenient to write guides' CUE packages and have them
			// import the preguide packages, for validation etc. That is to say,
			// if we _didn't_ import the preguide packages as part of a guide's
			// CUE package we would not be able to validate, code complete etc
			// independent of running preguide itself (which isn't ideal)
			//
			// TODO: remove use of a lock once cuelang.org/issues/460 is resolved
			m.Lock()
			err = modInit(env.WorkDir)
			m.Unlock()
			check(err, "failed to mod init: %v", err)

			// Always generate an output directory to save typing in each script
			outDir := filepath.Join(env.WorkDir, "_output")
			err = os.Mkdir(outDir, 0777)
			check(err, "failed to create %v: %v", outDir, err)

			return nil
		},
	}
	if err := gotooltest.Setup(&p); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, p)
}

func startserver(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("startserver cannot be negated")
	}
	var files []string
	fs := flag.NewFlagSet("startserver", flag.ContinueOnError)
	fs.Var(stringFlagList{&files}, "f", "file to compile into server")
	if err := fs.Parse(args); err != nil {
		ts.Fatalf("failed to parse startserver args: %v", err)
	}
	if len(files) == 0 {
		ts.Fatalf("need at least one -f flag value")
	}
	for i, f := range files {
		files[i] = ts.MkAbs(f)
	}
	td, err := ioutil.TempDir("", "")
	ts.Check(err)
	if err != nil {
		ts.Fatalf("failed to create temp dir for server: %v", err)
	}
	server := filepath.Join(td, "server")
	build := exec.Command("go", "build", "-o", server)
	if withRace() {
		build.Args = append(build.Args, "-race")
	}
	build.Args = append(build.Args, files...)
	if out, err := build.CombinedOutput(); err != nil {
		ts.Fatalf("failed to build server from %v: %v\n%s", files, err, out)
	}
	var serverAddress string
	done := make(chan struct{})
	errs := make(chan error)
	r, w := io.Pipe()
	var stderr bytes.Buffer
	cmd := exec.Command(server, fs.Args()...)
	cmd.Dir = ts.MkAbs(".")
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		ts.Fatalf("failed to start [%v]: %v", cmd, err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			errs <- (fmt.Errorf("got a non-zero exit code from [%v]: %v\n%s", cmd, err, stderr.Bytes()))
		} else {
			errs <- nil
		}
	}()
	ts.Defer(func() {
		// Ignore any errors on signalling...otherwise we can't get
		// errors from a process that has already died
		cmd.Process.Signal(os.Interrupt)
		if err := <-errs; err != nil {
			ts.Fatalf("got error on startserver errs channel: %v", err)
		}
	})
	// Read the first line of the command output as the server address
	go func() {
		scanner := bufio.NewScanner(r)
		line := 1
		for scanner.Scan() {
			if line == 1 {
				serverAddress = scanner.Text()
				close(done)
			}
			line++
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
	}()
	<-done

	ts.Setenv("PRESTEP_SERVER_BINARY", server)
	ts.Setenv("PRESTEP_SERVER_ADDRESS", serverAddress)
}

func createdockernetwork(ts *testscript.TestScript, neg bool, args []string) {
	// Create a docker network for the prestep docker test
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, time.Now().UnixNano())
	h := sha256.Sum256(b.Bytes())
	prestepNetwork := fmt.Sprintf("preguide-script-%x", h)
	cmd := exec.Command("docker", "network", "create", prestepNetwork)
	if out, err := cmd.CombinedOutput(); err != nil {
		ts.Fatalf("failed to run [%v]: %v\n%s", cmd, err, out)
	}
	ts.Defer(func() {
		cmd := exec.Command("docker", "network", "rm", prestepNetwork)
		if out, err := cmd.CombinedOutput(); err != nil {
			ts.Fatalf("failed to run [%v]: %v\n%s", cmd, err, out)
		}
	})
	ts.Setenv("PRESTEP_NETWORK", prestepNetwork)
}

// buildSelf builds the current package into a temporary directory. The path to
// the resulting binary is then returned.
func buildSelf(t *testing.T) string {
	td, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp dir for self build: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", td)
	if withRace() {
		cmd.Args = append(cmd.Args, "-race")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to run [%v]: %v\n%s", cmd, err, out)
	}
	t.Cleanup(func() {
		os.RemoveAll(td)
	})
	return filepath.Join(td, "preguide")
}

// modInitLock is used whilst the CUE API is not concurrency safe
var modInitLock sync.Mutex

// modInit establishes a temporary CUE module in dir and ensures the preguide
// CUE packages are vendored within that module
func modInit(dir string) (err error) {
	modInitLock.Lock()
	defer modInitLock.Unlock()

	defer util.HandleKnown(&err)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("%v is not a directory: %v", dir, err)
	}
	modDir := filepath.Join(dir, "cue.mod")
	err = os.Mkdir(modDir, 0777)
	check(err, "failed to mkdir %v: %v", modDir, err)

	modFile := filepath.Join(modDir, "module.cue")
	err = ioutil.WriteFile(modFile, []byte("module: \"mod.com\"\n"), 0666)
	check(err, "failed to write module file %v: %v", modFile, err)

	// TODO this approach is not particularly robust. But doesn't really matter
	// because with CUE modules the problem will go away
	bps := load.Instances([]string{"github.com/play-with-go/preguide", "github.com/play-with-go/preguide/out"}, nil)
	for _, bp := range bps {
		pkgRootBits := []string{modDir, "pkg"}
		pkgRootBits = append(pkgRootBits, strings.Split(bp.Module, "/")...)
		pkgRoot := filepath.Join(pkgRootBits...)

		for _, f := range bp.BuildFiles {
			fn := strings.TrimPrefix(f.Filename, bp.Root+string(os.PathSeparator))
			sfp := f.Filename
			tfp := filepath.Join(pkgRoot, fn)
			td := filepath.Dir(tfp)

			err = os.MkdirAll(td, 0777)
			check(err, "failed to create directory %v: %v", td, err)

			tf, err := os.Create(tfp)
			check(err, "failed to create %v: %v", tfp, err)

			sf, err := os.Open(sfp)
			check(err, "failed to open %v: %v", sfp, err)

			_, err = io.Copy(tf, sf)
			check(err, "failed to copy from %v to %v: %v", sfp, tfp, err)

			err = tf.Close()
			check(err, "failed to close %v: %v", tfp, err)
		}
	}
	return nil
}

// envsubst expands environment variable references in a file with the value of
// the current testscript environment.
func envsubst(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("envsubst does not support negation of the command")
	}
	if len(args) == 0 {
		ts.Fatalf("need to supply at least one filename")
	}

	for _, f := range args {
		f = ts.MkAbs(f)
		fc := ts.ReadFile(f)
		fc = os.Expand(fc, func(v string) string {
			return ts.Getenv(v)
		})
		ts.Check(ioutil.WriteFile(f, []byte(fc), 0666))
	}
}

func cmdCmpRegex() int {
	log.SetPrefix("")
	log.SetFlags(0)

	args := os.Args[1:]
	if len(args) != 2 {
		log.Fatalf("need to supply exactly two file names, GOT and WANT (could be stdout|stderr)")
	}

	gotName := args[0]
	wantName := args[1]
	got, err := ioutil.ReadFile(gotName)
	if err != nil {
		log.Fatal(err)
	}
	want, err := ioutil.ReadFile(wantName)
	if err != nil {
		log.Fatal(err)
	}

	c := cmpregexImpl{
		gotName:  gotName,
		wantName: wantName,
		got:      string(got),
		want:     string(want),
		Logf:     log.Printf,
		Fatalf:   log.Fatalf,
		Getenv:   os.Getenv,
	}
	c.Do()

	return 0
}

func cmpregex(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("cmpregex does not support negation of the command")
	}
	if len(args) != 2 {
		ts.Fatalf("need to supply exactly two file names, GOT and WANT (could be stdout|stderr)")
	}
	gotName := args[0]
	wantName := args[1]
	c := cmpregexImpl{
		gotName:  gotName,
		wantName: wantName,
		got:      ts.ReadFile(gotName),
		want:     ts.ReadFile(wantName),
		Logf:     ts.Logf,
		Fatalf:   ts.Fatalf,
		Getenv:   ts.Getenv,
	}
	c.Do()
}

type cmpregexImpl struct {
	gotName  string
	wantName string
	got      string
	want     string
	Logf     func(string, ...interface{})
	Fatalf   func(string, ...interface{})
	Getenv   func(string) string
}

func (c cmpregexImpl) Do() {
	got := c.got
	parts := splitAroundVars(c.want)
	normalised := ""
	isName := false
	for _, p := range parts {
		if !isName {
			if !strings.HasPrefix(got, p) {
				got = normalised + got
				want := normalised + p
				// Add trailing newlines for diff to work
				if len(got) > 0 && got[len(got)-1] != '\n' {
					got += "\n"
				}
				if len(want) > 0 && want[len(want)-1] != '\n' {
					want += "\n"
				}
				c.Logf("[diff -%s +%s]\n%s\n", c.gotName, c.wantName, textutil.Diff(got, want, true, nil, nil, nil))
				c.Fatalf("%s and %s differ", c.gotName, c.wantName)
			}
			normalised += p
			got = got[len(p):]
		} else {
			// We have a name - compile a regex from the env var value
			expr := c.Getenv(p)
			r, err := regexp.Compile(expr)
			if err != nil {
				c.Fatalf("failed to compile regex from env var %q value %q: %v", p, expr, err)
			}

			loc := r.FindStringIndex(got)
			if loc == nil {
				c.Logf("[finding regex %v]\n%s\n", r, got)
				c.Fatalf("failed to find regex from env var %q", p)
			}
			if loc[0] != 0 {
				c.Logf("[finding regex %v]\n%s\n", r, got)
				c.Fatalf("failed to find regex from env var %q at start of scope", p)
			}
			normalised += fmt.Sprintf("${%v}", p)
			got = got[loc[1]:]
		}
		isName = !isName
	}
}
