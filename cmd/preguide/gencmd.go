// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/kr/pretty"
	"github.com/play-with-go/preguide"
	"github.com/play-with-go/preguide/internal/types"
	"github.com/play-with-go/preguide/internal/util"
	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

// genCmd defines the gen command of preguide. This is where the main work
// happens.  gen operates on guides defined in directories within a working
// directory. Each directory d defines a different guide. A guide is composed
// of markdown files of prose, one per language, that reference directives
// defined in a CUE package located in d. The markdown files and CUE package
// represent the input to the gen command. The gen command is configured by
// CUE-specified configuration that follows the #PrestepServiceConfig schema.
//
// The gen command processes each directory d in turn. gen tries to evaluate a
// CUE package ./d/out to determine whether a guide's steps need to be re-run.
//
// The Go types for the input to the gen command are to be found in
// github.com/play-with-go/preguide/internal/types; they correspond to the
// definitions found in the github.com/play-with-go/preguide CUE package.  The
// Go types that represent the output of the gen command are to be found in the
// github.com/play-with-go/preguide Go package; they correspond to the
// definitions found in the github.com/play-with-go/preguide/out CUE package.
//
// TODO a note on Go types vs CUE definitions
//
// Ideally the Go types would be the source of truth for all the CUE
// definitions used by preguide (this comment is attached to genCmd on the
// basis the bulk of the work happens in this command). However, the story in
// converting from Go interface types to CUE definitions is "not yet complete",
// per github.com/cuelang/cue/discussions/462.
//
// Hence for now we adopt the simple approach of dual-maintaining Go types and
// CUE definitions for configuration types
// (github.com/play-with-go/preguide/internal/types.PrestepServiceConfig ->
// github.com/play-with-go/preguide.#PrestepServiceConfig), the input types
// (github.com/play-with-go/preguide/internal/types ->
// github.com/play-with-go/preguide) and the output types
// (github.com/play-with-go/preguide -> github.com/play-with-go/preguide/out).
// Theoretically we could code generate from
// internal/types.PrestepServiceConfig -> #PrestepServiceConfig, however it's
// not really worth it given we are dual-maintaining the rest already.
//
// Given that we cannot automatically extract CUE definitions from Go types,
// there is then a gap when it comes to runtime validation of
// config/input/output CUE.  This is explored in
// github.com/cuelang/cue/discussions/463. The crux of the problem is: how do
// we load the CUE definitions that correspond to the data we want to validate,
// given we can't derive those definitions from Go types (because if we could
// we would likely have been able to code generate from a Go source of truth in
// the first place). Our (temporary) answer to this problem is to embed the
// github.com/play-with-go/preguide and github.com/play-with-go/preguide/out
// CUE packages using go-bindata.
type genCmd struct {
	*runner
	fs *flag.FlagSet

	// See the initialisation of the flag fields for comments on their purpose

	fDir           *string
	flagDefaults   string
	fConfigs       []string
	fOutput        *string
	fSkipCache     *bool
	fImageOverride *string
	fPullImage     *string
	fDocker        *bool
	fPackage       *string
	fDebugCache    *bool
	fRun           *string
	fRunArgs       []string
	fMode          types.Mode

	fParallel *int

	// dir is the absolute path of the working directory specified by -dir
	// (if specified)
	dir string

	// config is parse configuration that results from unifying all the provided
	// config (which can be multiple CUE inputs)
	config preguide.PrestepServiceConfig

	// versionChecks is a map from pkg name to a channel used
	// to control waiting for the result of a version check
	versionChecks     map[string]chan struct{}
	versionChecksLock sync.Mutex

	// versions is a map from pkg to the resolved version
	// returned by the endpoint for that package
	versions     map[string]string
	versionsLock sync.Mutex

	// cueLock ensures we only ever have a single thread running CUE
	// code
	cueLock sync.Mutex
}

// getVersion returns the current version returned by the endpoint configured
// to serve package pkg. If pkg does not resolve to a service config, an error
// is raised.
func (pdc *processDirContext) getVersion(pkg string) string {
	pdc.versionsLock.Lock()
	v, ok := pdc.versions[pkg]
	pdc.versionsLock.Unlock()
	if ok {
		return v
	}

	// We do not have a version, check if we have a request
	// in flight
	pdc.versionChecksLock.Lock()
	c, ok := pdc.versionChecks[pkg]
	if !ok {
		c = make(chan struct{})
		pdc.versionChecks[pkg] = c
	}
	pdc.versionChecksLock.Unlock()
	if ok {
		<-c
		// We know that version will be available this time
		return pdc.getVersion(pkg)
	}

	// Get the version
	conf, ok := pdc.config[pkg]
	if !ok {
		raise("no config found for prestep %v", pkg)
	}
	var version string
	if conf.Endpoint.Scheme == "file" {
		version = "file"
	} else {
		version = string(pdc.doRequest("GET", conf.Endpoint.String()+"?get-version=1", conf))
	}
	pdc.versionsLock.Lock()
	pdc.versions[pkg] = version
	pdc.versionsLock.Unlock()
	close(c)

	return version
}

type processDirContext struct {
	*genCmd
	guideDir string

	guide *guide

	// The following is context that current sits on genCmd but
	// will likely have to move to a separate context object when
	// we start to concurrently process guides
	sanitiserHelper *sanitisers.S
	stmtPrinter     *syntax.Printer
}

func (pdc *processDirContext) debugf(format string, args ...interface{}) {
	format = fmt.Sprintf("%v: %v", pdc.relpath(pdc.guide.dir), format)
	pdc.runner.debugf(format, args...)
}

func newGenCmd(r *runner) *genCmd {
	res := &genCmd{
		runner:        r,
		fMode:         types.ModeJekyll,
		versions:      make(map[string]string),
		versionChecks: make(map[string]chan struct{}),
	}
	res.flagDefaults = newFlagSet("preguide gen", func(fs *flag.FlagSet) {
		res.fs = fs
		fs.Var(stringFlagList{&res.fConfigs}, "config", "CUE-style configuration input; can appear multiple times. See 'cue help inputs'")
		res.fDir = fs.String("dir", "", "the directory within which to run preguide")
		res.fOutput = fs.String("out", "", "the target directory for generation. If no value is specified it defaults to the input directory")
		res.fSkipCache = fs.Bool("skipcache", os.Getenv("PREGUIDE_SKIP_CACHE") == "true", "whether to skip any output cache checking")
		res.fImageOverride = fs.String("image", os.Getenv("PREGUIDE_IMAGE_OVERRIDE"), "the image to use instead of the guide-specified image")
		res.fPullImage = fs.String("pull", os.Getenv("PREGUIDE_PULL_IMAGE"), "try and docker pull image if missing")
		res.fDocker = fs.Bool("docker", false, "internal flag: run prestep requests in a docker container")
		res.fPackage = fs.String("package", "", "the CUE package name to use for the generated guide structure file")
		res.fDebugCache = fs.Bool("debugcache", false, "write a human-readable time-stamp-named file of the guide cache check to the current directory")
		res.fRun = fs.String("run", envOrVal("PREGUIDE_RUN", "."), "regexp that describes which guides within dir to validate and run")
		fs.Var(stringFlagList{&res.fRunArgs}, "runargs", "additional arguments to pass to the script that runs for a terminal. Format -run=$terminalName=args...; can appear multiple times")
		fs.Var(&res.fMode, "mode", fmt.Sprintf("the output mode. Valid values are: %v, %v, %v", types.ModeJekyll, types.ModeGitHub, types.ModeRaw))
		res.fParallel = fs.Int("parallel", 0, "allow parallel execution of preguide scripts. The value of this flag is the maximum number of scripts to run simultaneously. By default it is set to the value of GOMAXPROCS")
	})
	return res
}

// envOrVal evaluates environment variable e. If that variable is defined in
// the environment its value is returned, else v is returned.
func envOrVal(e string, v string) string {
	ev, ok := os.LookupEnv(e)
	if ok {
		return ev
	}
	return v
}

func (g *genCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide gen

%s`[1:], g.flagDefaults)
}

func (gc *genCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), gc}
}

// runGen is the implementation of the gen command.
func (gc *genCmd) run(args []string) error {
	var err error
	if err := gc.fs.Parse(args); err != nil {
		return gc.usageErr("failed to parse flags: %v", err)
	}
	dir := *gc.fDir
	dirArgs := gc.fs.Args()
	gotDir := dir != ""
	gotArgs := len(dirArgs) > 0

	// We set the default value of -parallel above to be zero so that we don't show a default
	// value. Reason being, the value of runtime.GOMAXPROCS(0) is system dependent. Which means
	// that tests that verify the output of -help are system dependent. This is bad. Hence
	// we need to distinguish the case where -parallel has been provided with a value of 0
	// or it has not (in which case we need to assign the default value)
	parallelSet := false
	gc.genCmd.fs.Visit(func(f *flag.Flag) {
		if f.Name == "parallel" {
			parallelSet = true
		}
	})
	if !parallelSet {
		v := runtime.GOMAXPROCS(0)
		gc.fParallel = &v
	}

	// TODO: pending whilst we await cuelang.org/go/... to become thread-safe
	if *gc.fParallel < 1 {
		return gc.usageErr("invalid value for -parallel; must be > 0")
	}
	if gotDir && gotArgs {
		return gc.usageErr("-dir and args are mutually exclusive")
	}
	if !gotDir && !gotArgs {
		gotDir = true
		dir = "."
	}
	if gotDir {
		gc.dir, err = filepath.Abs(dir)
		check(err, "failed to derive absolute directory from %q: %v", *gc.fDir, err)
	}

	runRegex, err := regexp.Compile(*gc.fRun)
	check(err, "failed to compile -run regex %q: %v", *gc.fRun, err)

	// Fallback to env-supplied config if no values supplied via -config flag
	if len(gc.fConfigs) == 0 {
		envVals := strings.Split(os.Getenv("PREGUIDE_CONFIG"), ":")
		for _, v := range envVals {
			v = strings.TrimSpace(v)
			if v != "" {
				gc.fConfigs = append(gc.fConfigs, v)
			}
		}
	}

	gc.runner.schemas, err = preguide.LoadSchemas(&gc.runtime)
	check(err, "failed to load schemas: %v", err)

	gc.loadConfig()

	// Any args to gen are considered directories to walk
	var toWalk []string
	switch {
	case gotArgs:
		for _, a := range gc.fs.Args() {
			fp, err := filepath.Abs(a)
			check(err, "failed to make arg absolute: %v", err)
			fi, err := os.Stat(fp)
			check(err, "failed to stat arg %v: %v", a, err)
			if !fi.IsDir() {
				raise("arg %v is not a directory", a)
			}
			toWalk = append(toWalk, fp)
		}
	case gotDir:
		// Read the source directory and process each guide (directory)
		dir, err := filepath.Abs(*gc.fDir)
		check(err, "failed to make path %q absolute: %v", *gc.fDir, err)
		es, err := ioutil.ReadDir(dir)
		check(err, "failed to read directory %v: %v", dir, err)
		for _, e := range es {
			if !e.IsDir() {
				continue
			}
			// Like cmd/go we skip hidden dirs
			if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") || e.Name() == "testdata" {
				continue
			}
			// Check against -run regexp
			if !runRegex.MatchString(e.Name()) {
				continue
			}
			toWalk = append(toWalk, filepath.Join(dir, e.Name()))
		}
	}
	// TODO: pending whilst we await cuelang.org/go/... to become thread-safe
	concurrentyLimit := *gc.fParallel
	// concurrentyLimit := 1
	limiter := make(chan struct{}, concurrentyLimit)
	for i := 0; i < concurrentyLimit; i++ {
		limiter <- struct{}{}
	}
	var errs []error
	raiseIfErrs := func() {
		if len(errs) == 0 {
			return
		}
		var errBuf bytes.Buffer
		for _, err := range errs {
			fmt.Fprintf(&errBuf, "%v\n", err)
		}
		raise("%s", errBuf.Bytes())
	}
	var wg sync.WaitGroup
	var resLock sync.Mutex
	var guides []*guide
	var pdcs []*processDirContext
	for _, d := range toWalk {
		pdc := &processDirContext{
			genCmd:          gc,
			sanitiserHelper: sanitisers.NewS(),
			stmtPrinter:     syntax.NewPrinter(syntax.SingleLine(true)),
			guideDir:        d,
		}
		pdcs = append(pdcs, pdc)
	}

	par := func(f func(pdc *processDirContext)) {
		for i := range pdcs {
			pdc := pdcs[i]
			<-limiter
			wg.Add(1)
			go func() {
				f(pdc)
				wg.Done()
				limiter <- struct{}{}
			}()
		}
	}
	par(func(pdc *processDirContext) {
		err := pdc.processDirPre(gotArgs)
		if err != nil {
			switch err.(type) {
			case errList:
				// We already have position information
			default:
				err = fmt.Errorf("%v: %v", pdc.relpath(pdc.guideDir), err)
			}
			resLock.Lock()
			errs = append(errs, err)
			resLock.Unlock()
		}
	})
	wg.Wait()
	raiseIfErrs()
	par(func(pdc *processDirContext) {
		if pdc.guide == nil {
			return
		}
		err := pdc.processDirPost()
		resLock.Lock()
		if err != nil {
			err = fmt.Errorf("%v: %v", pdc.relpath(pdc.guideDir), err)
			errs = append(errs, err)
		} else if len(pdc.guide.mdFiles) > 0 {
			guides = append(guides, pdc.guide)
		}
		resLock.Unlock()
	})
	wg.Wait()
	raiseIfErrs()
	gc.guides = guides
	if gotDir {
		gc.writeGuideStructures()
	}
	return nil
}

// loadConfig loads the configuration that drives the gen command. This
// configuration is described by the PrestepServiceConfig type, which is
// maintained as the #PrestepServiceConfig CUE definition.
func (gc *genCmd) loadConfig() {
	if len(gc.fConfigs) == 0 {
		return
	}

	// res will hold the config result
	var res cue.Value

	bis := load.Instances(gc.fConfigs, nil)
	for i, bi := range bis {
		inst, err := gc.runtime.Build(bi)
		check(err, "failed to load config from %v: %v", gc.fConfigs[i], err)
		res = res.Unify(inst.Value())
	}

	res = gc.schemas.PrestepServiceConfig.UnifyAccept(res, gc.schemas.PrestepServiceConfig)
	err := res.Validate()
	check(err, "failed to validate input config: %v", err)

	// Now we can extract the config from the CUE
	err = gc.codec.Encode(res, &gc.config)
	check(err, "failed to decode config from CUE value: %v", err)

	// Now validate that we don't have any networks for file protocol endpoints
	for ps, conf := range gc.config {
		if conf.Endpoint.Scheme == "file" {
			if len(conf.Env) > 0 {
				raise("prestep %v defined a file scheme endpoint %v but provided additional environment variables [%v]", ps, conf.Endpoint, conf.Env)
			}
			if len(conf.Networks) > 0 {
				raise("prestep %v defined a file scheme endpoint %v but provided networks [%v]", ps, conf.Endpoint, conf.Networks)
			}
		}
	}
}

// processDirPre does initial processing for the guide (CUE package and
// markdown files) found in dir. See the documentation for genCmd for more
// details. This phase does not run any docker commands
func (pdc *processDirContext) processDirPre(mustContainGuide bool) (err error) {
	defer util.HandleKnown(&err)
	target := *pdc.fOutput
	if target == "" {
		target = pdc.guideDir
	}
	g := &guide{
		dir:    pdc.guideDir,
		name:   filepath.Base(pdc.guideDir),
		target: target,
		Steps:  make(map[string]step),
		varMap: make(map[string]string),
	}

	// A guide is established by the presence of a CUE package in a directory.
	// If there are no markdown files for that guide, that is an error. If
	// they are for a different language, that is an error. Etc

	if !pdc.loadAndValidateSteps(g, mustContainGuide) {
		return nil // No guide in this directory
	}

	// At this point we were able to load a CUE package, so this directory
	// must contain valid markdown files

	if !pdc.loadMarkdownFiles(g) {
		return nil // No guide in this directory
	}

	pdc.guide = g

	// If we are running in -raw mode, then we want to skip checking
	// the out CUE package in g.dir. If we are not running in -raw
	// mode, we do want to try and load the out CUE package; this is
	// in effect like the Go build cache check.
	if pdc.fMode != types.ModeRaw {
		pdc.loadOutput(false)
	}

	return pdc.validateStepAndRefDirs()
}

// processDirPost runs the Docker phase of processing for the guide (CUE
// package and markdown files) found in dir
func (pdc *processDirContext) processDirPost() (err error) {
	defer util.HandleKnown(&err)

	pdc.checkPresteps()

	pdc.runSteps()

	pdc.validateOutRefsDirs()

	pdc.writeGuideOutput()

	pdc.writeLog()

	return
}

func (pdc *processDirContext) runSteps() {
	g := pdc.guide
	// If we have any steps to run, build a bash file that represents the script
	// to run. Then check whether the hash representing the contents of the bash
	// file matches the hash in the out CUE package (i.e. the result of a
	// previous run of this guide). If the hash matches, we don't have anything
	// to do: the inputs are identical and hence (because guides should be
	// idempotent) the output would be the same.
	if len(g.Steps) == 0 {
		return
	}
	pdc.buildBashFile(g)
	out := g.outputGuide
	cacheHit := out != nil && out.Hash == g.Hash
	if !*pdc.fSkipCache && cacheHit {
		pdc.debugf("cache hit: will not re-run script\n")
		g.updateFromOutput(out)
		return
	}
	pdc.runBashFile(g)
	if cacheHit && pdc.comparisonEqual(g, out) {
		g.updateFromOutput(out)
	} else {
		pdc.writeOutPackage(g)
	}
	if pdc.fMode != types.ModeRaw {
		// This step can be made more efficient if we know there is not
		// anything else in the out package other than the generated data
		// written in the previous step
		pdc.loadOutput(true)
	}
}

func (pdc *processDirContext) comparisonEqual(regen, out *guide) bool {
	// At this point we know we had the same input, i.e. a cache hit.
	// So we can safely iterate each step and simply compare comparison
	// output
	for _, r := range regen.steps {
		o := out.Steps[r.name()]
		switch r := r.(type) {
		case *commandStep:
			o := o.(*commandStep)
			for j, rs := range r.Stmts {
				os := o.Stmts[j]
				if rs.ComparisonOutput != os.ComparisonOutput {
					return false
				}
			}
		}
	}
	return true
}

// loadMarkdownFiles loads the markdown files for a guide. Markdown
// files are named according to isMarkdown, e.g en.markdown.
func (pdc *processDirContext) loadMarkdownFiles(g *guide) bool {
	es, err := ioutil.ReadDir(g.dir)
	check(err, "failed to read directory %v: %v", g.dir, err)

	var errs errList

	for _, e := range es {
		if !e.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(g.dir, e.Name())
		// We only want non-"hidden" markdown files
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext, ok := isMarkdown(e.Name())
		if !ok {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ext)
		if !types.ValidLangCode(lang) {
			continue
		}
		// Is this a valid language with respect to the guide?
		guideLang := false
		for _, l := range g.langs {
			if l == lang {
				guideLang = true
				break
			}
		}
		if !guideLang {
			errs.Addf("%v: %q is not a valid language for this guide", pdc.relpath(path), lang)
		}
		g.mdFiles = append(g.mdFiles, pdc.buildMarkdownFile(g, path, types.LangCode(lang), ext))
	}
	if errs.Err() != nil {
		panic(util.KnownErr{Err: errs.Err()})
	}
	if len(g.mdFiles) == 0 {
		raise("failed to load markdown files")
	}
	return true
}

// loadAndValidateSteps loads the CUE package for a guide and ensures that
// package is a valid instance of github.com/play-with-go/preguide.#Guide.
// Essentially this step involves loading CUE via the input types defined
// in github.com/play-with-go/preguide/internal/types, and results in g
// being primed with steps, terminals etc that represent a guide.
func (pdc *processDirContext) loadAndValidateSteps(g *guide, mustContainGuide bool) bool {
	locked := false
	lock := func() {
		pdc.cueLock.Lock()
		locked = true
	}
	unlock := func() {
		if locked {
			locked = false
			pdc.cueLock.Unlock()
		}
	}
	lock()
	defer unlock()
	conf := &load.Config{
		Dir: g.dir,
	}
	bps := load.Instances([]string{"."}, conf)
	gp := bps[0]
	if gp.Err != nil {
		if _, ok := gp.Err.(*load.NoFilesError); !mustContainGuide && ok {
			// absorb this error - we have nothing to do
			return false
		}
		check(gp.Err, "failed to load CUE package in %v: %v", g.dir, gp.Err)
	}

	gi, err := pdc.runtime.Build(gp)
	check(err, "failed to build %v: %v", gp.ImportPath, err)

	g.instance = gi

	// gv is the value that represents the guide's CUE package
	gv := gi.Value()

	// Double-check (because we are not guaranteed that the guide author) has
	// enforced this themselves that the package satisfies the #Steps schema
	//
	// We derive dv here because default values will be available via that
	// where required, but will not have source information (which is required
	// below)
	gv = pdc.schemas.Guide.UnifyAccept(gv, pdc.schemas.Guide)
	err = gv.Validate()
	if err != nil {
		var errstr strings.Builder
		errors.Print(&errstr, err, nil)
		raise("%v does not satisfy github.com/play-with-go/preguide.#Guide: %v", gp.ImportPath, errstr.String())
	}

	var intGuide types.Guide
	err = gv.Decode(&intGuide)
	check(err, "failed to decode guide: %T %v", err, err)

	g.langs = intGuide.Languages
	g.Delims = intGuide.Delims
	g.Networks = intGuide.Networks
	g.Env = intGuide.Env
	for _, s := range intGuide.Scenarios {
		g.Scenarios = append(g.Scenarios, s)
	}

	type termPosition struct {
		name string
		pos  token.Pos
	}
	var termPositions []termPosition
	if len(intGuide.Terminals) > 1 {
		raise("we only support a single terminal currently")
	}
	for n := range intGuide.Terminals {
		path := cue.MakePath(cue.Str("Terminals"), cue.Str(n))
		termPositions = append(termPositions, termPosition{
			name: n,
			pos:  structPos(gv.LookupPath(path)),
		})
	}
	sort.Slice(termPositions, func(i, j int) bool {
		return posLessThan(termPositions[i].pos, termPositions[j].pos)
	})
	for _, tp := range termPositions {
		n := tp.name
		t := intGuide.Terminals[n]
		g.Terminals = append(g.Terminals, t)
	}

	// Before we release the CUE lock, grab the positions of the steps
	// which we will use for later sorting
	type stepPosition struct {
		name string
		pos  token.Pos
	}
	var stepPositions []stepPosition
	for stepName := range intGuide.Steps {
		stepPositions = append(stepPositions, stepPosition{
			name: stepName,
			pos:  structPos(gi.Lookup("Steps", stepName)),
		})
	}
	unlock()

	// Create presteps - but we will check them later
	for _, prestep := range intGuide.Presteps {
		ps := guidePrestep{
			Package: prestep.Package,
			Path:    prestep.Path,
			Args:    prestep.Args,
		}
		if ps.Package == "" {
			raise("Prestep had empty package")
		}
		g.Presteps = append(g.Presteps, &ps)
	}

	for stepName, v := range intGuide.Steps {
		var s step
		switch is := v.(type) {
		case *types.Command:
			s, err = pdc.commandStepFromCommand(is)
			check(err, "failed to parse #Command from step %v: %v", stepName, err)
		case *types.CommandFile:
			if !filepath.IsAbs(is.Path) {
				is.Path = filepath.Join(g.dir, is.Path)
			}
			s, err = pdc.commandStepFromCommandFile(is)
			check(err, "failed to parse #CommandFile from step %v: %v", stepName, err)
		case *types.Upload:
			// TODO: when we support non-Unix terminals,
			s, err = pdc.uploadStepFromUpload(is)
			check(err, "failed to parse #Upload from step %v: %v", stepName, err)
		case *types.UploadFile:
			if !filepath.IsAbs(is.Path) {
				is.Path = filepath.Join(g.dir, is.Path)
			}
			s, err = pdc.uploadStepFromUploadFile(is)
			check(err, "failed to parse #UploadFile from step %v: %v", stepName, err)
		}
		// Validate various things about the step
		switch s := s.(type) {
		case *uploadStep:
			// TODO: this check needs to be made platform specific, specific
			// to the platform on which it will run (which is determined
			// by the terminal scenario). However for now we assume Unix
			if !isAbsolute(s.Target) {
				raise("target path %q must be absolute", s.Target)
			}
		}
		g.Steps[stepName] = s
	}

	sort.Slice(g.langs, func(i, j int) bool {
		return g.langs[i] < g.langs[j]
	})

	// TODO: error on steps for multiple languages until we support
	// github.com/play-with-go/preguide/issues/64
	if len(g.langs) > 0 && (len(g.langs) > 2 || g.langs[0] != "en") {
		raise("we only support steps for English language guides for now")
	}

	// TODO: error on multiple scenarios until we support
	// github.com/play-with-go/preguide/issues/64
	if len(g.Scenarios) > 1 {
		raise("we only support a single scenario for now")
	}

	// Sort according to the order of the steps as declared in the
	// guide [filename, offset]
	sort.Slice(stepPositions, func(i, j int) bool {
		return posLessThan(stepPositions[i].pos, stepPositions[j].pos)
	})
	for i, sp := range stepPositions {
		s := g.Steps[sp.name]
		g.steps = append(g.steps, s)
		s.setorder(i)
	}
	return true
}

func (pdc *processDirContext) checkPresteps() {
	g := pdc.guide

	// We only investigate the presteps if we have any steps
	// to run
	if len(g.Steps) == 0 {
		return
	}
	for _, ps := range g.Presteps {
		ps.Version = pdc.getVersion(ps.Package)
	}
}

// loadOutput attempts to load the out CUE package. Each successful run of
// preguide writes this package for multiple reasons. It is a human readable
// log of the input to the guide steps, the commands that were run, the output
// from those commands etc. But it also acts as a "build cache" in that the
// hash of the various inputs to a guide is also written to this package. That
// way, if a future run of preguide sees the same inputs, then the running of
// the steps can be skipped because the output will be the same (guides are
// meant to be idempotent). This massively speeds up the guide writing process.
//
// The out package also has a human-defined element to it. outref's can be defined
// to make referring to specific parts of the generated output simpler.
//
// The full parameter indicates whether we require the load of the full out
// package, or only the generated part. When we are looking to determine
// whether to re-run the steps of a guide or not, the out package may not exist
// (first time that preguide has been run for example), or it might not be
// valid (a definition might have been added that references a part of the
// generated output that does not yet exist). Hence we only attempt to load
// the generated part of the out package. When we come to later verify
// any outref's, the fully package must be loaded
//
// When we are not loading the full package, we also tolerate errors. This
// simply has the side effect of not setting the output guide or output guide
// instance.
func (pdc *processDirContext) loadOutput(full bool) {
	g := pdc.guide
	pdc.cueLock.Lock()
	defer pdc.cueLock.Unlock()

	var err error

	conf := &load.Config{
		Dir: g.dir,
	}
	toLoad := outPkg
	// TODO: when we drop the sanityCheck call below (i.e. when we fix
	// cuelang.org/issue/567) then we can re-enable this code that restricts
	// the initial load to the generated part of the package
	// if !full {
	// 	toLoad = path.Join(toLoad, genOutCueFile)
	// }
	toLoad = "./" + toLoad
	bps := load.Instances([]string{toLoad}, conf)
	gp := bps[0]
	if !full && gp.Err != nil {
		return
	}
	check(gp.Err, "failed to load out CUE package from %v: %v", toLoad, gp.Err)

	gi, err := pdc.runtime.Build(gp)
	if !full && err != nil {
		return
	}
	check(err, "failed to build %v: %v", gp.ImportPath, err)

	// gv is the value that represents the guide's CUE package
	gv := gi.Value()

	err = pdc.schemas.GuideOutput.UnifyAccept(gv, (pdc.schemas.GuideOutput)).Validate()
	if !full && err != nil {
		return
	}
	check(err, "failed to validate %v against out schema: %v", gp.ImportPath, err)

	var out guide
	err = gv.Decode(&out)
	if !full && err != nil {
		return
	}
	check(err, "failed to decode Guide from out value: %v", errors.Details(err, &errors.Config{Cwd: g.dir}))

	// Set g.steps = nil because at this stage we know we have
	// valid information in g.Steps
	g.steps = nil
	for _, step := range g.Steps {
		g.steps = append(g.steps, step)
	}
	sort.Slice(g.steps, func(i, j int) bool {
		return g.steps[i].order() < g.steps[j].order()
	})

	g.outputGuide = &out
	g.outinstance = gi
}

// validateStepAndRefDirs ensures that step (e.g. <!-- step: step1 -->) and
// reference (e.g. <!-- ref: world -->) directives in the guide's markdown
// files are valid. That is, they resolve to either a named step of a reference
// directive. Out reference directives (e.g. <!-- outref: cmdoutput -->) are
// checked later (once we are guaranteed the out CUE package exists).
func (pdc *processDirContext) validateStepAndRefDirs() error {
	pdc.cueLock.Lock()
	defer pdc.cueLock.Unlock()

	g := pdc.guide
	// TODO: verify that we have identical sets of languages when we support
	// multiple languages
	var errs errList

	for _, mdf := range g.mdFiles {
		mdf.frontMatter[guideFrontMatterKey] = g.name

		var stepDirectivesToCheck []*stepDirective

		// First ensure that we reference existent steps and references
		// are correct.
		for _, d := range mdf.directives {
			switch d := d.(type) {
			case *stepDirective:
				_, found := g.Steps[d.name]
				if !found {
					errs.Addf("%v:%v: unknown step %q referened", pdc.relpath(mdf.path), d.Pos(), d.name)
				}
				stepDirectivesToCheck = append(stepDirectivesToCheck, d)
			case *refDirective:
				if g.instance == nil {
					// This should never really happen so raise as an error
					raise("found a ref directive %v but no CUE instance?", d.String())
				}
				sels := []cue.Selector{cue.Str("Defs")}
				sels = append(sels, d.path.Selectors()...)
				path := cue.MakePath(sels...)
				v := g.instance.Value().LookupPath(path)
				if err := v.Err(); err != nil {
					errs.Addf("%v:%v: failed to evaluate {%v}: %v", pdc.relpath(mdf.path), d.Pos(), d.String(), err)
					continue
				}
				switch v.Kind() {
				case cue.StringKind:
				default:
					errs.Addf("%v:%v: value resulting from {%v} is of unsupported kind %v", pdc.relpath(mdf.path), d.Pos(), d.String(), v.Kind())
					continue
				}
				d.val = v
			case *outrefDirective:
				// we don't validate this at this point
			default:
				panic(fmt.Errorf("don't yet know how to handle %T type", d))
			}
		}

		// Now ensure that we have the correct ordering (this is a more
		// terminal error, so we do this second). We know all the step
		// directives reference valid steps, so don't need to check for
		// the iteration through the step directives going beyond the end
		// of the guide steps slice.

		// nextStep keeps track of which step we are comparing against
		// from the guide script. It essentially represents the expectaion
		// on the next step. Once we have iterated through all the
		// directives for a markdown file, if any guide script steps remain
		// then these are errors.
		//
		// Notice below that we skipped
		badOrder := false
		stepsToCheck := g.steps
	stepDirectives:
		for _, stepDir := range stepDirectivesToCheck {
			if len(stepsToCheck) == 0 {
				// This can only happen when we have valid stepDirectives (i.e. they resolve
				// to a step) that occur after the expected sequence of steps. i.e. these are superfluous
				// step directives. Add errors for each one
				errs.Addf("%v:%v: saw superfluous step directive %v", pdc.relpath(mdf.path), stepDir.Pos(), stepDir.name)
				continue stepDirectives
			}
			for {
				s := stepsToCheck[0]
				stepsToCheck = stepsToCheck[1:]
				// Regardless of whether we need to reference this step or not,
				// if we do, move on to the next directive
				if s.name() == stepDir.name {
					continue stepDirectives
				}
				// We know the directive name and step name do not match. If
				// we must reference this step, then that is an error.
				if s.mustBeReferenced() {
					badOrder = true
					errs.Addf("%v:%v: saw step directive %v; expected to see %v", pdc.relpath(mdf.path), stepDir.Pos(), stepDir.name, s.name())
					// This is a fairly terminal error... because the order of everything
					// that follows is likely to be off as a result
					break stepDirectives
				}
				// Continue looping over the guide script steps
			}
		}
		if !badOrder {
			missingLine := 1 + bytes.Count(mdf.source, []byte("\n"))
			// Now walk through the remainder of the steps and add
			// errors indicating they are not reference
			for _, s := range stepsToCheck {
				if s.mustBeReferenced() {
					errs.Addf("%v:%v: step %q was not referenced", pdc.relpath(mdf.path), missingLine, s.name())
				}
			}
		}
	}

	return errs.Err()
}

type errList []error

func (l *errList) Add(err error) {
	*l = append(*l, err)
}

func (l *errList) Addf(format string, args ...interface{}) {
	l.Add(fmt.Errorf(format, args...))
}

func (l errList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

// Error is implemented so that errList implements error
func (l errList) Error() string {
	if len(l) == 0 {
		return "nil"
	}
	var buf bytes.Buffer
	var prefix string
	for _, e := range l {
		fmt.Fprintf(&buf, "%v%v", prefix, e)
		prefix = "\n"
	}
	return buf.String()
}

// validateOutRefsDirs ensures that outref directives (e.g. <!-- outref:
// cmdoutput -->) are valid (step and ref directives were checked earlier).
// This second pass of checking the outrefs specifically is required because
// only at this stage in the processing of a guide can we be guaranteed that
// the out package exists (and hence any outref directives) resolve.
func (pdc *processDirContext) validateOutRefsDirs() {
	g := pdc.guide
	for _, mdf := range g.mdFiles {
		for _, d := range mdf.directives {
			switch d := d.(type) {
			case *stepDirective:
			case *refDirective:
			case *outrefDirective:
				if g.outinstance == nil {
					raise("found an outref directive %v but no out CUE instance?", d.String())
				}
				sels := []cue.Selector{cue.Str("Defs")}
				sels = append(sels, d.path.Selectors()...)
				path := cue.MakePath(sels...)
				v := g.outinstance.Value().LookupPath(path)
				if err := v.Err(); err != nil {
					raise("failed to evaluate %v: %v", d.String(), err)
				}
				switch v.Kind() {
				case cue.StringKind:
				default:
					raise("value at %v is of unsupported kind %v", d.String(), v.Kind())
				}
				d.val = v
				// we don't validate this at this point
			default:
				panic(fmt.Errorf("don't yet know how to handle %T type", d))
			}
		}
	}
}

const (
	outPkg        = "out"
	genOutCueFile = "gen_out.cue"
)

func (pdc *processDirContext) writeOutPackage(g *guide) {
	pdc.cueLock.Lock()
	defer pdc.cueLock.Unlock()
	enc := gocodec.New(&pdc.runner.runtime, nil)
	v, err := enc.Decode(g)
	check(err, "failed to decode guide to CUE: %v", err)
	syn := v.Syntax()
	sl := sortSteps(syn)
	out, err := valueToFile(outPkg, sl)
	check(err, "failed to format CUE output: %v", err)

	// If we are in raw mode we dump output to stdout. It's more of a debugging mode
	if pdc.fMode == types.ModeRaw {
		fmt.Printf("%s", out)
		return
	}

	outDir := filepath.Join(g.dir, outPkg)
	err = os.MkdirAll(outDir, 0777)
	check(err, "failed to mkdir %v: %v", outDir, err)
	outFilePath := filepath.Join(outDir, genOutCueFile)
	err = ioutil.WriteFile(outFilePath, []byte(out), 0666)
	check(err, "failed to write output to %v: %v", outFilePath, err)
}

func (pdc *processDirContext) runBashFile(g *guide) {
	// Now run the pre-step if there is one
	var toWrite string
	for _, ps := range g.Presteps {
		// TODO: run the presteps concurrently, but add their args in order
		// last prestep's args last etc

		var jsonBody []byte

		// At this stage we know we have a valid endpoint (because we previously
		// checked it via a get-version=1 request)
		conf := pdc.config[ps.Package]
		if conf.Endpoint.Scheme == "file" {
			if ps.Args != nil {
				raise("prestep %v (path %v) provides arguments [%v]: but prestep is configured with a file endpoint", ps.Package, ps.Path, pretty.Sprint(ps.Args))
			}
			// Notice this path takes no account of the -docker flag
			var err error
			path := conf.Endpoint.Path
			jsonBody, err = ioutil.ReadFile(path)
			check(err, "failed to read file endpoint %v (file %v): %v", conf.Endpoint, path, err)
		} else {
			u := *conf.Endpoint
			u.Path = path.Join(u.Path, ps.Path)
			jsonBody = pdc.doRequest("POST", u.String(), conf, ps.Args)
		}

		// TODO: unmarshal jsonBody into a cue.Value, validate against a schema
		// for valid prestep results then decode via gocodec into out (below)

		var out struct {
			Vars []string
		}
		err := json.Unmarshal(jsonBody, &out)
		check(err, "failed to unmarshal output from prestep %v: %v\n%s", ps.Package, err, jsonBody)
		for _, v := range out.Vars {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				raise("bad env var received from prestep: %q", v)
			}
			g.vars = append(g.vars, v)
			g.varMap[parts[0]] = parts[1]
			ps.Variables = append(ps.Variables, parts[0])
		}
	}
	// If we have any vars we need to first perform an expansion of any
	// templates instances {{.ENV}} that appear in the bashScript, and then
	// append the result of that substitution. Note this substitution applies
	// to both the commands AND the uploads
	bashScript := g.bashScript
	if len(g.vars) > 0 {
		t := template.New("pre-substitution bashScript")
		t.Delims(g.Delims[0], g.Delims[1])
		t.Option("missingkey=error")
		_, err := t.Parse(bashScript)
		check(err, "failed to parse pre-substitution bashScript: %v", err)
		var b bytes.Buffer
		err = t.Execute(&b, g.varMap)
		check(err, "failed to execute pre-substitution bashScript template: %v", err)
		bashScript = b.String()
	}

	// Concatenate the bash script
	toWrite += bashScript

	// Create a temp directory for our "workings". Note, this directory will
	// not be world-readable by default. So when it comes to the directory we
	// wil mount inside the docker container that will run the script, we
	// need to be a bit more liberal with permissions. Doing so within the
	// the temp directory is safe.
	td, err := ioutil.TempDir("", fmt.Sprintf("preguide-%v-runner-", g.name))
	check(err, "failed to create workings directory for guide %v: %v", g.dir, err)
	defer os.RemoveAll(td)

	scriptsDir := filepath.Join(td, "scripts")
	err = os.Mkdir(scriptsDir, 0777)
	check(err, "failed to create scripts directory %v: %v", scriptsDir, err)
	scriptsFile := filepath.Join(scriptsDir, "script.sh")
	err = ioutil.WriteFile(scriptsFile, []byte(toWrite), 0777)
	check(err, "failed to write temporary script to %v: %v", scriptsFile, err)

	// Explicitly change the permissions for the scripts directory and the
	// script itself so that when mounted within the docker container they are
	// runnable by anyone. This is necessary because the bind mount used adopts
	// the same owner and permissions as the host. Therefore, to be runnable
	// by any user, including the user who ends up running the script as defined
	// by the image we are using, we need to be liberal.
	err = os.Chmod(scriptsDir, 0777)
	check(err, "failed to change permissions of %v: %v", scriptsDir, err)
	err = os.Chmod(scriptsFile, 0777)
	check(err, "failed to change permissions of %v: %v", scriptsFile, err)

	// Whilst we know we have a single terminal, we can use the g.Image() hack
	// of finding the image for that single terminal. We we support multiple
	// terminals we will need to move away from that hack
	image := g.Image()
	if *pdc.fImageOverride != "" {
		image = *pdc.fImageOverride
	}

	// Whilst we know we have a single terminal, we know we can also safely
	// address the single terminal's name for the purposes of checking our
	// -runargs flag values
	term := g.Terminals[0]
	var termRunArgs []string
	for _, a := range pdc.fRunArgs {
		var err error
		p := term.Name + "="
		if !strings.HasPrefix(a, p) {
			raise("bad argument passed to -runargs, does not correspond to terminal: %q", a)
		}
		v := strings.TrimPrefix(a, p)
		termRunArgs, err = split(v)
		check(err, "failed to split -runargs in words: %v; value was %q", err, v)
	}

	imageCheck := exec.Command("docker", "inspect", image)
	out, err := imageCheck.CombinedOutput()
	if err != nil {
		if *pdc.fPullImage == pullImageMissing {
			pdc.debugf("failed to find docker image %v (%v); will attempt pull\n", image, err)
			pull := exec.Command("docker", "pull", image)
			out, err = pull.CombinedOutput()
			check(err, "failed to find docker image %v; also failed to pull it: %v\n%s", image, err, out)
		} else {
			raise("failed to find docker image %v (%v); either pull this image manually or use -pull=missing", image, err)
		}
	}

	cmd := pdc.newDockerRunner(g.Networks,
		"--rm",
		"-t", // otherwise stderr is not line buffered
		"-v", fmt.Sprintf("%v:/scripts", scriptsDir),
	)
	cmd.Args = append(cmd.Args, termRunArgs...)
	for _, v := range g.vars {
		cmd.Args = append(cmd.Args, "-e", v)
	}
	for _, v := range g.Env {
		cmd.Args = append(cmd.Args, "-e", v)
	}
	cmd.Args = append(cmd.Args, image, "/scripts/script.sh")

	if os.Getenv("PREGUIDE_PROGRESS") == "true" {
		var outbuf bytes.Buffer
		pipeRead, pipeWrite := io.Pipe()
		cmd.Stdout = io.MultiWriter(&outbuf, pipeWrite)
		cmd.Stderr = cmd.Stdout
		pipeDone := make(chan error)
		go func() {
			s := bufio.NewScanner(pipeRead)
			for s.Scan() {
				fmt.Printf("%v: %v\n", pdc.relpath(g.dir), s.Text())
			}
			if err := s.Err(); err != nil && err != io.EOF {
				pipeDone <- err
			}
			close(pipeDone)
		}()
		err = cmd.Run()
		check(err, "failed to run [%v]: %v\n%s", strings.Join(cmd.Args, " "), err, out)
		err = pipeWrite.Close()
		check(err, "failed to close write pipe for [%v]: %v", strings.Join(cmd.Args, " "), cmd)
		err = <-pipeDone
		check(err, "failed to write output from [%v]: %v", strings.Join(cmd.Args, " "), err)
		out = outbuf.Bytes()
	} else {
		out, err = cmd.CombinedOutput()
		check(err, "failed to run [%v]: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}

	pdc.debugf("script output:\n%s", out)

	walk := out
	slurp := func(end []byte) (res string) {
		endI := bytes.Index(walk, end)
		if endI == -1 {
			raise("failed to find %q before end of output:\n%q\nOutput was: %q\n", end, walk, out)
		}
		res, walk = string(walk[:endI]), walk[endI+len(end):]
		// Because we are running in -t mode, replace all \r\n with \n
		res = strings.ReplaceAll(res, "\r\n", "\n")
		return res
	}

	// As we go through getting the output, continue to build up a list of
	// replacements that will sanitise the output in a second pass.
	//
	// First add the variables that are the result of the prestep.
	var sanVals [][2]string
	if pdc.fMode != types.ModeRaw {
		for name, val := range g.varMap {
			repl := g.Delims[0] + "." + name + g.Delims[1]
			sanVals = append(sanVals, [2]string{
				val, repl,
			})
		}
	}
	for _, step := range g.steps {
		switch step := step.(type) {
		case *commandStep:
			var stepOutput *bytes.Buffer
			doRandomReplace := pdc.fMode != types.ModeRaw && step.RandomReplace != nil
			if doRandomReplace {
				stepOutput = new(bytes.Buffer)
			}
			for _, stmt := range step.Stmts {
				// TODO: tidy this up
				fence := []byte(stmt.outputFence + "\r\n")
				slurp(fence) // Ignore everything before the fence
				stmt.Output = slurp(fence)
				if doRandomReplace {
					stepOutput.WriteString(stmt.Output)
				}
				exitCodeStr := slurp([]byte("\r\n"))
				stmt.ExitCode, err = strconv.Atoi(exitCodeStr)
				check(err, "failed to parse exit code from %q at position %v in output: %v\n%s", exitCodeStr, len(out)-len(walk)-len(exitCodeStr)-1, err, out)
			}
			if doRandomReplace {
				v := stepOutput.String()
				if !step.DoNotTrim {
					v = trimTrailingNewline(v)
				}
				sanVals = append(sanVals, [2]string{
					v, *step.RandomReplace,
				})
			}
		}
	}
	// Ensure we do not have any duplicate values to be sanitised
	// (we even error if we see the a value more than once with the
	// same replacement)
	sanKeys := make(map[string]int)
	for _, v := range sanVals {
		sanKeys[v[0]] = sanKeys[v[0]] + 1
	}
	var badSanVals []string
	for k, v := range sanKeys {
		if v > 1 {
			badSanVals = append(badSanVals, k)
		}
	}
	if len(badSanVals) > 0 {
		sort.Strings(badSanVals)
		raise("the following values to be sanitised had multiple replacements: %v", badSanVals)
	}
	// Now sort the sanitisation values into reverse length order i.e. longest
	// first, in case some sanitisationw values are substrings of longer ones
	sort.Slice(sanVals, func(i, j int) bool {
		lhs, rhs := sanVals[i], sanVals[j]
		return len(lhs[0]) > len(rhs[0])
	})
	// Now sanitise everything
	for _, step := range g.steps {
		switch step := step.(type) {
		case *commandStep:
			for _, stmt := range step.Stmts {
				o := stmt.Output
				for _, san := range sanVals {
					o = strings.ReplaceAll(o, san[0], san[1])
				}
				cmpO := o
				// Now run sanitisers
				if san := stmt.sanitiser; san != nil {
					o = san.Output(nil, o)
					cmpO = san.ComparisonOutput(nil, o)
				}
				stmt.Output = o
				stmt.ComparisonOutput = cmpO
			}
		}
	}
}

// buildBashFile creates a bash file to run for the language-specific steps of
// a guide.
func (pdc *processDirContext) buildBashFile(g *guide) {
	// TODO when we come to support multiple terminals this will need to be
	// rethought. Perhaps something along the following lines:
	//
	// * We in effect create a special bash script per terminal. But we don't
	// write that script and then run it in one shot, we instead use docker
	// run -i and have bash read from stdin. This way we can control the order
	// of events between terminals.
	//
	// * The stdout and stderr of the docker run process should be the same
	// value (per os/exec) so that we get correctly interleaved stdout and
	// stderr
	//
	// * The order of steps is defined by the natural source order of step
	// names. That is to say, the first time a step declaration is encountered
	// determines that step's position in the order of all steps
	//
	// * We only need to block a given terminal at the "edge" of handover
	// between terminals
	//
	// * We can determine the process ID of the special bash script by echo-ing
	// $$ at the start of the special script.
	//
	// * This is fine because it opens the door for us being able to supply
	// input over stdin should this ever be necessary
	//
	// * Question is how to deal with blocking calls, e.g. running an http
	// server. Support this initially by not allowing background processes or
	// builtins (because with builtins there is no child process). Then
	// interpreting a special kill or <Ctrl-c> command to kill the current
	// (because there can only be one) blocked process. Then continuing to the
	// next "block"
	//
	// * How to identify these blocking calls? Well, so long as we don't support
	// background processes then the next call _must_ be a <Ctrl-c> (even if
	// that is the last command in a script), otherwise that script cannot make
	// progress or return. This might be sufficient an indicator for now.
	//
	// * As part of this we should not expect a blocking call to echo the
	// final code fence until we kill it, and vice versa: a non-blocking call
	// should output the code fence (so we should wait until we see it)
	//

	var sb strings.Builder
	pf := func(format string, args ...interface{}) {
		fmt.Fprintf(&sb, format, args...)
	}
	h := sha256.New()
	var out io.Writer = h
	if *pdc.fDebugCache {
		now := time.Now().UTC()
		debugFileName := fmt.Sprintf("%v_%v_%v.txt", g.name, now.Format("20060102_150405"), now.Nanosecond())
		debugFile, err := os.OpenFile(debugFileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
		check(err, "failed to create cache debug file %v: %v", debugFileName, err)
		out = io.MultiWriter(out, debugFile)
	}
	hf := func(format string, args ...interface{}) {
		fmt.Fprintf(out, format, args...)
	}
	// Write the module info for github.com/play-with-go/preguide
	hf("preguide: %#v\n", pdc.versionString)
	// We write the Presteps information to the hash, and only run the pre-step
	// if we have a cache miss and come to run the bash file. Note that
	// this _includes_ the buildID (hence the use of pretty.Sprint rather
	// than JSON), whereas in the log we use JSON to _not_ include the
	// buildID
	hf("prestep: %s\n", mustJSONMarshalIndent(g.Presteps))
	// We write the docker image to the hash, because if the user want to ensure
	// reproducibility they should specify the full digest.
	hf("image: %v\n", g.Image())
	pf("#!/usr/bin/env -S bash -l\n")
	pf("export TERM=dumb\n")
	pf("export NO_COLOR=true\n")
	for _, step := range g.steps {
		switch step := step.(type) {
		case *commandStep:
			for i, stmt := range step.Stmts {
				hf("step: %q, command statement %v: %v\n\n", step.Name, i, stmt.CmdStr)
				// echo the command we will run
				cmdEchoFence := getFence()
				pf("cat <<'%v'\n", cmdEchoFence)
				pf("$ %v\n", stmt.CmdStr)
				pf("%v\n", cmdEchoFence)
				stmt.outputFence = getFence()
				pf("echo %v\n", stmt.outputFence)
				pf("%v\n", stmt.CmdStr)
				pf("x=$?\n")
				pf("echo %v\n", stmt.outputFence)
				if stmt.Negated {
					pf("if [ $x -eq 0 ]\n")
				} else {
					pf("if [ $x -ne 0 ]\n")
				}
				pf("then\n")
				pf("exit 1\n")
				pf("fi\n")
				pf("echo $x\n")
			}
		case *uploadStep:
			hf("step: %q, upload: target: %v, source: %v\n\n", step.Name, step.Target, step.Source)
			cmdEchoFence := getFence()
			pf("cat <<'%v'\n", cmdEchoFence)
			pf("$ cat <<EOD > %v\n", step.Target)
			pf("%v\n", step.Source)
			pf("EOD\n")
			pf("%v\n", cmdEchoFence)
			fence := getFence()
			pf("cat <<'%v' > %v\n", fence, step.Target)
			pf("%v\n", step.Source)
			pf("%v\n", fence)
			pf("x=$?\n")
			pf("if [ $x -ne 0 ]\n")
			pf("then\n")
			pf("exit 1\n")
			pf("fi\n")
		default:
			panic(fmt.Errorf("can't yet handle steps of type %T", step))
		}
	}
	pdc.debugf("bash script:\n%v", sb.String())
	g.bashScript = sb.String()
	g.Hash = fmt.Sprintf("%x", h.Sum(nil))
}

func getFence() string {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, time.Now().UnixNano())
	return fmt.Sprintf("%x", sha256.Sum256(b.Bytes()))
}

// structPos returns the position of the struct value v.
func structPos(v cue.Value) token.Pos {
	if v.Err() != nil {
		raise("asked to find struct position of error value: %v", v.Err())
	}
	_, parts := v.Expr()
	return parts[0].Pos()
}

func posLessThan(lhs, rhs token.Pos) bool {
	cmp := strings.Compare(lhs.Filename(), rhs.Filename())
	if cmp == 0 {
		cmp = lhs.Offset() - rhs.Offset()
	}
	return cmp < 0
}

// doRequest performs the an HTTP request according to the supplied parameters
// taking into account whether the -docker flag is set. args are JSON encoded.
//
// In the special case that url is a file protocol, args is expected to be zero
// length, and the -docker flag is ignored (that is to say, it is expected the
// file can be accessed by the current process).
func (pdc *processDirContext) doRequest(method string, endpoint string, conf *preguide.ServiceConfig, args ...interface{}) []byte {
	var body io.Reader
	if len(args) > 0 {
		var w bytes.Buffer
		enc := json.NewEncoder(&w)
		for i, arg := range args {
			err := enc.Encode(arg)
			check(err, "failed to encode arg %v (%v): %v", i, pretty.Sprint(arg), err)
		}
		body = &w
	}
	// We need Docker if we need to connect to networks
	if len(conf.Networks) > 0 && !*pdc.fDocker {
		cmd := pdc.newDockerRunner(conf.Networks,
			// Don't leave this container around
			"--rm",
		)
		for _, e := range conf.Env {
			cmd.Args = append(cmd.Args, "-e", e)
		}
		pdc.addSelfArgs(cmd)
		// Now add the arguments to "ourselves"
		cmd.Args = append(cmd.Args, "/runbin/preguide", "docker", method, endpoint)
		if body != nil {
			byts, err := ioutil.ReadAll(body)
			check(err, "failed to read from body: %v", err)
			cmd.Args = append(cmd.Args, string(byts))
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		check(err, "failed to docker run %v: %v\n%s", strings.Join(cmd.Args, " "), err, stderr.Bytes())

		return stdout.Bytes()
	}

	req, err := http.NewRequest(method, endpoint, body)
	check(err, "failed to build HTTP request for method %v, url %q: %v", method, endpoint, err)
	resp, err := http.DefaultClient.Do(req)
	check(err, "failed to perform HTTP request with args [%v]: %v", args, err)
	if resp.StatusCode/100 != 2 {
		raise("got non-success status code (%v) with args [%v]", resp.StatusCode, args)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	check(err, "failed to read response body for request %v: %v", req, err)
	return respBody
}

// addSelfArgs is ultimately responsible for adding the image that will be run
// as part of this docker command. However it also adds any supporting arguments
// e.g. like mounts
func (gc *genCmd) addSelfArgs(dr *dockerRunnner) {
	bi := gc.buildInfo
	if os.Getenv("PREGUIDE_DEVEL_IMAGE") != "true" && bi.Main.Replace == nil && bi.Main.Version != "(devel)" {
		dr.Args = append(dr.Args, fmt.Sprintf("playwithgo/preguide:%v", bi.Main.Version))
		return
	}
	dr.Args = append(dr.Args,
		fmt.Sprintf("--volume=%s:/runbin/preguide", gc.self),
		imageBase,
	)
}

type mdFile struct {
	path        string
	source      []byte
	content     []byte
	frontMatter map[string]interface{}
	frontFormat string
	lang        types.LangCode
	ext         string
	directives  []directive
}

type directive interface {
	Pos() position
	End() position
	String() string
}

type baseDirective struct {
	// pos is the position of the start of the directive
	pos position

	// end is the position of the end of the directive
	end position
}

func (b *baseDirective) Pos() position {
	return b.pos
}

func (b *baseDirective) End() position {
	return b.end
}

type stepDirective struct {
	*baseDirective
	name string
}

func (s stepDirective) String() string {
	return s.name
}

type refDirective struct {
	*baseDirective
	path cue.Path
	val  cue.Value
}

func (r refDirective) String() string {
	return r.path.String()
}

type outrefDirective struct {
	*baseDirective
	path cue.Path
	val  cue.Value
}

func (o outrefDirective) String() string {
	return o.path.String()
}

const (
	stepDirectiveName         = "step"
	refDirectiveName          = "ref"
	outrefDirectiveName       = "outref"
	dockerImageFrontMatterKey = "image"
	guideFrontMatterKey       = "guide"
	langFrontMatterKey        = "lang"
	scnearioFrontMatterKey    = "scenario"
)

func (pdc *processDirContext) buildMarkdownFile(g *guide, path string, lang types.LangCode, ext string) mdFile {
	source, err := ioutil.ReadFile(path)
	check(err, "failed to read %v: %v", path, err)

	// Check we have a frontmatter
	// TODO: drop this dependency and write our own simply parser
	front, err := pageparser.ParseFrontMatterAndContent(bytes.NewReader(source))
	check(err, "failed to parse front matter in %v: %v", path, err)
	content := front.Content
	if content == nil {
		// This appears to be a bug with the pageparser package
		content = source
	}

	// TODO: support all front-matter formats... and no front matter

	if _, ok := front.FrontMatter[langFrontMatterKey]; ok {
		raise("do not declare language via %q key in front matter", langFrontMatterKey)
	}
	if _, ok := front.FrontMatter[dockerImageFrontMatterKey]; ok {
		raise("do not declare docker image via %q key in front matter", dockerImageFrontMatterKey)
	}
	// Now set the lang of the markdown frontmatter based on the filename
	front.FrontMatter[langFrontMatterKey] = lang

	res := mdFile{
		path:        path,
		lang:        lang,
		ext:         ext,
		source:      source,
		content:     content,
		frontMatter: front.FrontMatter,
		frontFormat: string(front.FrontMatterFormat),
	}
	// Parse the directives accordign to the following constraints:
	//
	// 1. There can be no directives in the header section
	// 2. The only valid directives are {{{.ENV}}}, {{{ref "ENV"}}}, {{{step stepDirectiveName}}},
	// hence we will only ever have a list of nodes to walk.

	funcs := map[string]interface{}{
		outrefDirectiveName: true,
		refDirectiveName:    true,
		stepDirectiveName:   true,
	}
	contentStart := bytes.Index(source, content)
	parseTrees, err := parse.Parse("guide", string(content), g.Delims[0], g.Delims[1], funcs)
	check(err, "failed to parse markdown body: %v", err)
	tree := parseTrees["guide"]
	line := 1 + bytes.Count(source[:contentStart], []byte("\n")) // Safe because we know content is a substring
	lastOffset := 0
	var nl = []byte("\n")
	offsetToPosition := func(offset int) position {
		line += bytes.Count(content[lastOffset:offset], nl)
		col := offset
		if lastnl := bytes.LastIndex(content[:offset], nl); lastnl != -1 {
			col -= lastnl
		}
		lastOffset = offset
		return position{
			col:    col,
			line:   line,
			offset: offset,
		}
	}
	nodeToPosition := func(n parse.Node) position {
		// Calculate the offset, remembering that we need to work
		// backwards to the start of the directive
		offset := int(n.Position())
		switch n.(type) {
		case *parse.ActionNode:
			offset = bytes.LastIndex(content[:offset], []byte(g.Delims[0]))
		}
		return offsetToPosition(offset)
	}
	var lastEnd *position
	var errs errList
	for _, n := range tree.Root.Nodes {
		start := nodeToPosition(n)
		if lastEnd != nil {
			*lastEnd = start
			lastEnd = nil
		}
		switch n.(type) {
		case *parse.TextNode:
			continue
		case *parse.ActionNode:
		default:
			errs.Addf("%v:%v: expected to see action node; saw %T", pdc.relpath(path), start, n)
			continue
		}
		action := n.(*parse.ActionNode)
		if numCmds := len(action.Pipe.Cmds); numCmds != 1 {
			errs.Addf("%v:%v: expected to see a single command; saw %v", pdc.relpath(path), start, numCmds)
			continue
		}
		cmd := action.Pipe.Cmds[0]
		// At this point we can either have
		// .ENV
		// ref "ENV"
		// step stepDirectiveName
		var bd baseDirective
		lastEnd = &bd.end
		switch first := cmd.Args[0].(type) {
		case *parse.FieldNode:
			var sels []cue.Selector
			for _, f := range first.Ident {
				sels = append(sels, cue.Str(f))
			}
			path := cue.MakePath(sels...)
			bd.pos = start
			res.directives = append(res.directives, &refDirective{
				baseDirective: &bd,
				path:          path,
			})
		case *parse.IdentifierNode:
			rem := cmd.Args[1:]
			switch first.Ident {
			case refDirectiveName, outrefDirectiveName:
				var sels []cue.Selector
				for _, a := range rem {
					sn, ok := a.(*parse.StringNode)
					if !ok {
						errs.Addf("%v:%v: %v expected string arg; saw %T", pdc.relpath(path), start, first.Ident, a)
						continue
					}
					sels = append(sels, cue.Str(sn.Text))
				}
				path := cue.MakePath(sels...)
				bd.pos = start
				switch first.Ident {
				case refDirectiveName:
					res.directives = append(res.directives, &refDirective{
						baseDirective: &bd,
						path:          path,
					})
				case outrefDirectiveName:
					res.directives = append(res.directives, &outrefDirective{
						baseDirective: &bd,
						path:          path,
					})
				}
			case stepDirectiveName:
				if len(rem) != 1 {
					errs.Addf("%v:%v: step takes a single string argument; saw %v argument(s)", pdc.relpath(path), start, len(rem))
					continue
				}
				strArg, ok := rem[0].(*parse.StringNode)
				if !ok {
					errs.Addf("%v:%v: expected string argument; saw %T", pdc.relpath(path), start, rem[0])
					continue
				}
				bd.pos = start
				res.directives = append(res.directives, &stepDirective{
					baseDirective: &bd,
					name:          strArg.Text,
				})
			default:
				errs.Addf("%v:%v: unknown function %q", pdc.relpath(path), start, first.Ident)
				continue
			}
		default:
			errs.Addf("%v:%v: unexpected node %T", pdc.relpath(path), start, n)
			continue
		}
	}
	if lastEnd != nil {
		*lastEnd = offsetToPosition(len(content))
	}
	if errs.Err() != nil {
		panic(util.KnownErr{Err: errs.Err()})
	}
	return res
}

// writeGuideStructures writes an instance of
// github.com/play-with-go/preguide/out.#GuideStructures to the working
// directory. This config can then be used directly by a controller for the
// guides found in that directory.
func (gc *genCmd) writeGuideStructures() {
	structures := make(map[string]preguide.GuideStructure)
	for _, guide := range gc.guides {
		s := preguide.GuideStructure{
			Delims:    guide.Delims,
			Terminals: guide.Terminals,
			Networks:  guide.Networks,
			Scenarios: guide.Scenarios,
			Env:       guide.Env,
		}
		for _, ps := range guide.Presteps {
			s.Presteps = append(s.Presteps, &preguide.Prestep{
				Package: ps.Package,
				Path:    ps.Path,
				Args:    ps.Args,
			})
		}
		structures[guide.name] = s
	}
	v, err := gc.codec.Decode(structures)
	check(err, "failed to decode guide structures to CUE value: %v", err)
	// Now do a sanity check against the schema

	// TODO: remove roundtrip to syntax post fix for cuelang.org/issue/530
	vn, _ := format.Node(v.Syntax())
	i2, _ := gc.runtime.Compile("hello.cue", vn)
	v2 := i2.Value()

	err = gc.schemas.GuideStructures.UnifyAccept(v2, gc.schemas.GuideStructures).Validate()
	check(err, "failed to validate guide structures against schema: %v", err)
	pkgName := *gc.fPackage
	if pkgName == "" {
		pkgName = filepath.Base(gc.dir)
		pkgName = strings.ReplaceAll(pkgName, "-", "_")
	}
	syn, err := valueToFile(pkgName, v.Syntax())
	check(err, "failed to convert guide structures to CUE syntax: %v", err)
	outPath := filepath.Join(*gc.fDir, "gen_guide_structures.cue")
	err = ioutil.WriteFile(outPath, append(syn, '\n'), 0666)
	check(err, "failed to write guide structures output to %v: %v", outPath, err)
}

func sortSteps(n ast.Node) ast.Node {
	s := n.(*ast.StructLit)
	stepsField := structField(s, "Steps")
	if stepsField == nil {
		// This should never happen... because we are in control of the conversion
		panic(fmt.Errorf("failed to find Steps field in %v", pretty.Sprint(s)))
	}
	stepsVal := stepsField.Value.(*ast.StructLit)
	sort.Slice(stepsVal.Elts, func(i, j int) bool {
		lhs, rhs := stepsVal.Elts[i].(*ast.Field), stepsVal.Elts[j].(*ast.Field)
		lhsOrder := structField(lhs.Value, "Order").Value.(*ast.BasicLit)
		rhsOrder := structField(rhs.Value, "Order").Value.(*ast.BasicLit)
		lhsv, _ := strconv.Atoi(lhsOrder.Value)
		rhsv, _ := strconv.Atoi(rhsOrder.Value)
		return lhsv < rhsv
	})
	return s
}

func valueToFile(pkg string, n ast.Node) ([]byte, error) {
	s := n.(*ast.StructLit)
	f := &ast.File{}
	f.Decls = append(f.Decls, &ast.Package{
		Name: ast.NewIdent(pkg),
	})
	f.Decls = append(f.Decls, s.Elts...)
	return format.Node(f)
}

// structField finds field within the struct literal decl
func structField(decl ast.Decl, field string) *ast.Field {
	sl := decl.(*ast.StructLit)
	for _, el := range sl.Elts {
		switch el := el.(type) {
		case *ast.Field:
			switch l := el.Label.(type) {
			case *ast.Ident:
				if l.Name == field {
					return el
				}
			}
		}
	}
	return nil
}

// dockerRunnner is a convenience type used to wrap the three call
// dance required to run a docker container with multiple
// networks attached
type dockerRunnner struct {
	pdc *processDirContext

	// DockerArgs are the flags to pass to each docker command
	DockerArgs []string

	// Env is the environment passed to each docker command
	Env []string

	Path     string
	Args     []string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Networks []string
}

func (pdc *processDirContext) newDockerRunner(networks []string, args ...string) *dockerRunnner {
	return &dockerRunnner{
		pdc:      pdc,
		Networks: append([]string{}, networks...),
		Args:     append([]string{}, args...),
	}
}

func (dr *dockerRunnner) Run() error {
	createCmd := exec.Command("docker", "create")
	createCmd.Env = dr.Env
	createCmd.Args = append(createCmd.Args, dr.Args...)
	var createStdout, createStderr bytes.Buffer
	createCmd.Stdout = &createStdout
	createCmd.Stderr = &createStderr

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed %v: %v\n%s", createCmd, err, createStderr.Bytes())
	}

	instance := strings.TrimSpace(createStdout.String())

	for _, network := range dr.Networks {
		connectCmd := exec.Command("docker", "network", "connect", network, instance)
		if out, err := connectCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed %v: %v\n%s", connectCmd, err, out)
		}
	}

	startCmd := exec.Command("docker", "start", "-a", instance)
	startCmd.Stdin = dr.Stdin
	startCmd.Stdout = dr.Stdout
	startCmd.Stderr = dr.Stderr

	return startCmd.Run()
}

func (dr *dockerRunnner) CombinedOutput() ([]byte, error) {
	if dr.Stdout != nil {
		return nil, fmt.Errorf("cmd Stdout already set")
	}
	if dr.Stderr != nil {
		return nil, fmt.Errorf("cmd Sderr already set")
	}
	var comb bytes.Buffer
	dr.Stdout = &comb
	dr.Stderr = &comb
	err := dr.Run()
	return comb.Bytes(), err
}
