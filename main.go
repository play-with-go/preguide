// preguide is a pre-processor for Play With Docker-based guides
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/gohugoio/hugo/parser/pageparser"
	"golang.org/x/net/html"
)

type runner struct {
	*rootCmd
	genCmd  *genCmd
	initCmd *initCmd
	helpCmd *helpCmd

	runtime cue.Runtime

	dir string

	codec *gocodec.Codec

	guideDef       cue.Value
	commandDef     cue.Value
	commandFileDef cue.Value
	uploadDef      cue.Value
	uploadFileDef  cue.Value
	guideOutDef    cue.Value
	commandStep    cue.Value
	uploadStep     cue.Value
}

func newRunner() *runner {
	return &runner{
		dir: ".",
	}
}

func (r *runner) mainerr() (err error) {
	defer handleKnown(&err)

	if err := r.rootCmd.fs.Parse(os.Args[1:]); err != nil {
		return usageErr{err, r.rootCmd}
	}

	args := r.rootCmd.fs.Args()
	if len(args) == 0 {
		return r.rootCmd.usageErr("missing command")
	}
	cmd := args[0]
	switch cmd {
	case "gen":
		return r.runGen(args[1:])
	case "init":
		return r.runInit(args[1:])
	case "help":
		return r.runHelp(args[1:])
	default:
		return r.rootCmd.usageErr("unknown command: " + cmd)
	}
}

func (r *runner) runHelp(args []string) error {
	if len(args) != 1 {
		return r.helpCmd.usageErr("help takes a single command argument")
	}
	var u func() string
	switch args[0] {
	case "gen":
		u = r.genCmd.usage
	case "init":
		u = r.initCmd.usage
	case "help":
		u = r.rootCmd.usage
	default:
		return r.helpCmd.usageErr("no help available for command %v", args[0])
	}
	fmt.Print(u())
	return nil
}
func (r *runner) runInit(args []string) error {
	return nil
}

func (r *runner) runGen(args []string) error {
	if err := r.genCmd.fs.Parse(args); err != nil {
	}
	if r.genCmd.fOutput == nil || *r.genCmd.fOutput == "" {
		return r.genCmd.usageErr("target directory must be specified")
	}

	r.codec = gocodec.New(&r.runtime, nil)
	r.loadSchemas()

	dir, err := filepath.Abs(r.dir)
	check(err, "failed to make path %q absolute: %v", r.dir, err)

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
		r.processDir(filepath.Join(dir, e.Name()))
	}
	return nil
}

func (r *runner) debugf(format string, args ...interface{}) {
	if *r.fDebug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func (r *runner) loadSchemas() {
	pkgs := []string{"github.com/play-with-go/preguide", "github.com/play-with-go/preguide/out"}
	bps := load.Instances(pkgs, nil)
	var insts []*cue.Instance
	for i, pkg := range pkgs {
		check(bps[i].Err, "failed to load %v: %v", pkg, bps[i].Err)
		inst, err := r.runtime.Build(bps[i])
		check(err, "failed to build %v: %v", pkg, err)
		insts = append(insts, inst)
	}
	preguide, preguideOut := insts[0], insts[1]

	r.guideDef = preguide.LookupDef("#Guide")
	r.commandDef = preguide.LookupDef("#Command")
	r.commandFileDef = preguide.LookupDef("#CommandFile")
	r.uploadDef = preguide.LookupDef("#Upload")
	r.uploadFileDef = preguide.LookupDef("#UploadFile")
	r.guideOutDef = preguideOut.LookupDef("#GuideOutput")
	r.commandStep = preguideOut.LookupDef("#CommandStep")
	r.uploadStep = preguideOut.LookupDef("#UploadStep")
}

func (r *runner) processDir(dir string) {

	g := &guide{
		dir:    dir,
		name:   filepath.Base(dir),
		target: *r.genCmd.fOutput,
		Langs:  make(map[string]*langSteps),
	}

	r.loadMarkdownFiles(g)
	if len(g.mdFiles) == 0 {
		return
	}

	r.loadSteps(g)
	r.loadOutput(g, false)

	stepCount := r.validateStepAndRefDirs(g)

	if stepCount > 0 {
		outputLoadRequired := false
		for _, l := range g.langs {
			ls := g.Langs[l]
			r.buildBashFile(g, ls)
			if !*r.genCmd.fSkipCache {
				if out := g.outputGuide; out != nil {
					if ols := out.Langs[l]; ols != nil {
						if ols.Hash == ls.Hash {
							r.debugf("cache hit for %v: will not re-run script\n", l)
							ls.Steps = ols.Steps
							ls.steps = ols.steps
							continue
						}
					}
				}
			}
			outputLoadRequired = true
			r.runBashFile(g, ls)
		}
		r.writeOutput(g)
		if outputLoadRequired || g.outputGuide == nil {
			r.loadOutput(g, true)
		}
	}

	r.validateOutRefsDirs(g)

	r.process(g)
	r.generateTestLog(g)
}

func (r *runner) validateStepAndRefDirs(g *guide) (stepCount int) {
	// TODO: verify that we have identical sets of languages when we support
	// multiple languages

	stepDirCount := 0
	for _, mdf := range g.mdFiles {
		if g.Image != "" {
			mdf.frontMatter[dockerImageFrontMatterKey] = g.Image
		}

		// TODO: improve language steps fallback
		ls, ok := g.Langs[mdf.lang]
		if !ok {
			ls = g.Langs["en"]
		}
		for _, d := range mdf.directives {
			switch d := d.(type) {
			case *stepDirective:
				stepDirCount++
				var found bool
				found = ls != nil
				if found {
					found = ls.Steps != nil
				}
				if found {
					_, found = ls.Steps[d.key]
				}
				if !found {
					raise("unknown step %q referened in file %v", d.key, mdf.path)
				}
			case *refDirective:
				if g.instance == nil {
					raise("found a ref directive %v but not CUE instance?", d.key)
				}
				key := "Defs." + d.key
				expr, err := parser.ParseExpr("dummy", key)
				check(err, "failed to parse CUE expression from %q: %v", key, err)
				v := g.instance.Eval(expr)
				if err := v.Err(); err != nil {
					raise("failed to evaluate %v: %v", key, err)
				}
				switch v.Kind() {
				case cue.StringKind:
				default:
					raise("value at %v is of unsupported kind %v", key, v.Kind())
				}
				d.val = v
			case *outrefDirective:
				// we don't validate this at this point
			default:
				panic(fmt.Errorf("don't yet know how to handle %T type", d))
			}
		}
	}
	for _, ls := range g.Langs {
		stepCount += len(ls.steps)
	}
	if stepDirCount == 0 && stepCount > 0 {
		fmt.Fprintln(os.Stderr, "This guide does not have any directives but does have steps to run.")
		fmt.Fprintln(os.Stderr, "Not running those steps because they are not referenced.")
	}
	return stepCount
}

func (r *runner) validateOutRefsDirs(g *guide) {
	for _, mdf := range g.mdFiles {
		for _, d := range mdf.directives {
			switch d := d.(type) {
			case *stepDirective:
			case *refDirective:
			case *outrefDirective:
				if g.outinstance == nil {
					raise("found an outref directive %v but no out CUE instance?", d.key)
				}
				key := "Defs." + d.key
				expr, err := parser.ParseExpr("dummy", key)
				check(err, "failed to parse CUE expression from %q: %v", key, err)
				v := g.outinstance.Eval(expr)
				if err := v.Err(); err != nil {
					raise("failed to evaluate %v: %v", key, err)
				}
				switch v.Kind() {
				case cue.StringKind:
				default:
					raise("value at %v is of unsupported kind %v", key, v.Kind())
				}
				d.val = v
				// we don't validate this at this point
			default:
				panic(fmt.Errorf("don't yet know how to handle %T type", d))
			}
		}
	}
}

func (r *runner) writeOutput(g *guide) {
	outDir := filepath.Join(g.dir, "out")
	err := os.MkdirAll(outDir, 0777)
	check(err, "failed to mkdir %v: %v", outDir, err)
	enc := gocodec.New(&r.runtime, nil)
	v, err := enc.Decode(g)
	check(err, "failed to decode guide to CUE: %v", err)
	byts, err := format.Node(v.Syntax())
	out := fmt.Sprintf("package out\n\n%s", byts)
	check(err, "failed to format CUE output: %v", err)
	outFilePath := filepath.Join(outDir, "gen_out.cue")
	err = ioutil.WriteFile(outFilePath, []byte(out), 0666)
	check(err, "failed to write output to %v: %v", outFilePath, err)
}

func (r *runner) loadOutput(g *guide, fail bool) {
	conf := &load.Config{
		Dir: g.dir,
	}
	bps := load.Instances([]string{"./out"}, conf)
	gp := bps[0]
	if gp.Err != nil {
		if fail {
			raise("failed to load out CUE package from %v: %v", filepath.Join(g.dir, "out"), gp.Err)
		}
		// absorb this error - we have nothing to do. We will fix
		// out when we write the output later (all being well)
		return
	}

	gi, err := r.runtime.Build(gp)
	if err != nil {
		if fail {
			raise("failed to build %v: %v", gp.ImportPath, err)
		}
		return
	}

	// gv is the value that represents the guide's CUE package
	gv := gi.Value()

	if err := gv.Unify(r.guideOutDef).Validate(); err != nil {
		if fail {
			raise("failed to validate %v against out schema: %v", gp.ImportPath, err)
		}
		return
	}

	var out guide
	r.encodeGuide(gv, &out)

	g.outputGuide = &out
	g.output = gv
	g.outinstance = gi
}

func (r *runner) runBashFile(g *guide, ls *langSteps) {
	// Now run the pre-step if there is one
	var toWrite string
	if g.PreStep.Package != "" {
		args := []string{"go", "run", "-exec", fmt.Sprintf("go run mvdan.cc/dockexec %v", *r.genCmd.fPreStepDockExec), g.PreStep.Package}
		args = append(args, g.PreStep.Args...)
		var stdout, stderr bytes.Buffer
		prestep := exec.Command(args[0], args[1:]...)
		prestep.Stdout = &stdout
		prestep.Stderr = &stderr
		err := prestep.Run()
		check(err, "failed to run prestep [%#v]: %v\n%s", prestep.Args, err, stderr.Bytes())
		var out struct {
			Script string
			Vars   map[string]string
		}
		err = json.Unmarshal(stdout.Bytes(), &out)
		check(err, "failed to unmarshal output from prestep: %v\n%s", err, stdout.Bytes())
		var vars [][2]string
		for k, v := range out.Vars {
			vars = append(vars, [2]string{k, v})
		}
		sort.SliceStable(vars, func(i, j int) bool {
			lhs, rhs := vars[i], vars[j]
			if lhs[1] == rhs[1] {
				raise("prestep defined vars that map to same value: %v and %v", lhs[0], rhs[0])
			}
			return strings.Contains(lhs[1], rhs[1])
		})
		g.vars = vars
		toWrite = out.Script + "\n"
	}
	// Concatenate the bash script
	toWrite += ls.bashScript
	td, err := ioutil.TempDir("", fmt.Sprintf("preguide-%v-runner-", g.name))
	check(err, "failed to create temp directory for guide %v: %v", g.dir, err)
	defer os.RemoveAll(td)
	sf := filepath.Join(td, "script.sh")
	err = ioutil.WriteFile(sf, []byte(toWrite), 0777)
	check(err, "failed to write temporary script to %v: %v", sf, err)

	imageCheck := exec.Command("docker", "inspect", g.Image)
	out, err := imageCheck.CombinedOutput()
	if err != nil {
		if *r.genCmd.fPullImage == pullImageMissing {
			r.debugf("failed to find docker image %v (%v); will attempt pull", g.Image, err)
			pull := exec.Command("docker", "pull", g.Image)
			out, err = pull.CombinedOutput()
			check(err, "failed to find docker image %v; also failed to pull it: %v\n%s", g.Image, err, out)
		} else {
			raise("failed to find docker image %v (%v); either pull this image manually or use -pull=missing", g.Image, err)
		}
	}

	cmd := exec.Command("docker", "run", "--rm",
		"-v", fmt.Sprintf("%v:/scripts", td),
		"-e", fmt.Sprintf("USER_UID=%v", os.Geteuid()),
		"-e", fmt.Sprintf("USER_GID=%v", os.Getegid()),
		g.Image, "/scripts/script.sh",
	)
	out, err = cmd.CombinedOutput()
	check(err, "failed to run [%v]: %v\n%s", strings.Join(cmd.Args, " "), err, out)

	r.debugf("output from [%v]:\n%s", strings.Join(cmd.Args, " "), out)

	walk := out
	slurp := func(end []byte) (res string) {
		endI := bytes.Index(walk, end)
		if endI == -1 {
			raise("failed to find %q before end of output:\n%s", end, out)
		}
		res, walk = string(walk[:endI]), walk[endI+len(end):]
		return res
	}

	for _, step := range ls.steps {
		switch step := step.(type) {
		case *commandStep:
			for _, stmt := range step.Stmts {
				// TODO: tidy this up
				fence := []byte(stmt.outputFence + "\n")
				if !bytes.HasPrefix(walk, fence) {
					raise("failed to find %q at position %v in output:\n%s", stmt.outputFence, len(out)-len(walk), out)
				}
				walk = walk[len(fence):]
				stmt.RawOutput = slurp(fence)
				stmt.Output = stmt.RawOutput
				for _, s := range append(stmt.sanitisers, g.sanitiseVars) {
					stmt.Output = s(stmt.Output)
				}
				exitCodeStr := slurp([]byte("\n"))
				stmt.ExitCode, err = strconv.Atoi(exitCodeStr)
				check(err, "failed to parse exit code from %q at position %v in output: %v\n%s", exitCodeStr, len(out)-len(walk)-len(exitCodeStr)-1, err, out)
			}
		}
	}
}

func (r *runner) encodeGuide(v cue.Value, g *guide) {
	// TODO: pending a solution to cuelang.org/issue/377 we do this
	// by hand
	g.Image, _ = v.Lookup("Image").String()
	g.PreStep.Package, _ = v.Lookup("PreStep", "Package").String()
	g.PreStep.buildID, _ = v.Lookup("PreStep", "BuildID").String()
	g.PreStep.Version, _ = v.Lookup("PreStep", "Version").String()
	{
		args := v.Lookup("PreStep", "Args")
		_ = args.Decode(&g.PreStep.Args)
	}
	if g.Langs == nil {
		g.Langs = make(map[string]*langSteps)
	}
	iter, _ := v.Lookup("Langs").Fields()
	for iter.Next() {
		lang := iter.Label()
		v := iter.Value()
		ls := &langSteps{}
		r.encodeLangSteps(v, ls)
		g.Langs[lang] = ls
	}
}

func (r *runner) encodeLangSteps(v cue.Value, ls *langSteps) {
	ls.Hash, _ = v.Lookup("Hash").String()
	if ls.Steps == nil {
		ls.Steps = make(map[string]step)
	}
	iter, _ := v.Lookup("Steps").Fields()
	for iter.Next() {
		stepName := iter.Label()
		v := iter.Value()
		var s step
		switch {
		case r.commandStep.Unify(v).Equals(v):
			s = &commandStep{}
		case r.uploadStep.Unify(v).Equals(v):
			s = &uploadStep{}
		default:
			raise("failed to match type of %v", v)
		}
		err := r.codec.Encode(v, s)
		check(err, "failed to re-encode %v for %v", err, v)
		ls.Steps[stepName] = s
		ls.steps = append(ls.steps, s)
	}
	sort.Slice(ls.steps, func(i, j int) bool {
		return ls.steps[i].order() < ls.steps[j].order()
	})
}

func (r *runner) buildBashFile(g *guide, ls *langSteps) {
	var sb strings.Builder
	pf := func(format string, args ...interface{}) {
		fmt.Fprintf(&sb, format, args...)
	}
	h := sha256.New()
	hf := func(format string, args ...interface{}) {
		fmt.Fprintf(h, format, args...)
	}
	// We write the PreStep information to the hash, and only run the pre-step if
	// we have a cache miss and come to run the bash file
	hf("prestep: %#v", g.PreStep)
	// We write the docker image to the hash, because if the user want to ensure
	// reproducibility they should specify the full digest.
	hf("image: %v\n", g.Image)
	pf("#!/usr/bin/env bash\n")
	for _, step := range ls.steps {
		switch step := step.(type) {
		case *commandStep:
			for i, stmt := range step.Stmts {
				hf("step: %q, command statement %v: %v\n\n", step.Name, i, stmt.CmdStr)
				var b bytes.Buffer
				binary.Write(&b, binary.BigEndian, time.Now().UnixNano())
				h := sha256.Sum256(b.Bytes())
				stmt.outputFence = fmt.Sprintf("%x", h)
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
			var b bytes.Buffer
			binary.Write(&b, binary.BigEndian, time.Now().UnixNano())
			fence := fmt.Sprintf("%x", sha256.Sum256(b.Bytes()))
			pf("cat <<%v > %v\n", fence, step.Target)
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
	r.debugf("Bash script:\n%v", sb.String())
	ls.bashScript = sb.String()
	ls.Hash = fmt.Sprintf("%x", h.Sum(nil))
}

func (r *runner) loadSteps(g *guide) {
	conf := &load.Config{
		Dir: g.dir,
	}
	bps := load.Instances([]string{"."}, conf)
	gp := bps[0]
	if gp.Err != nil {
		if _, ok := gp.Err.(*load.NoFilesError); ok {
			// absorb this error - we have nothing to do
			return
		}
		check(gp.Err, "failed to load CUE package in %v: %T", g.dir, gp.Err)
	}

	gi, err := r.runtime.Build(gp)
	check(err, "failed to build %v: %v", gp.ImportPath, err)

	g.instance = gi

	// gv is the value that represents the guide's CUE package
	gv := gi.Value()

	// Does the guide CUE package validate?
	err = gv.Validate(cue.Final(), cue.Concrete(true))
	check(err, "%v does not validate: %v", gp.ImportPath, err)

	// Double-check (because we are not guaranteed that the guide author) has
	// enforced this themselves that the package satisfies the #Steps schema
	err = r.guideDef.Unify(gv).Validate()
	check(err, "%v does not satisfy github.com/play-with-go/preguide.#Steps: %v", gp.ImportPath, err)

	if stepsV := gv.Lookup("Steps"); stepsV.Exists() {
		steps, _ := stepsV.Struct()

		g.Image, _ = gv.Lookup("Image").String()
		if *r.genCmd.fImageOverride != "" {
			g.Image = *r.genCmd.fImageOverride
		}
		if g.Image == "" {
			raise("Image not specified, but we have steps to run")
		}
		g.PreStep.Package, _ = gv.Lookup("PreStep", "Package").String()
		{
			args := gv.Lookup("PreStep", "Args")
			_ = args.Decode(&g.PreStep.Args)
		}
		if g.PreStep.Package != "" {
			// TODO: cache this step; package lookup will not change over the course
			// of running a guide

			// Resolve to a version. The runner of preguide should be doing
			// so in the context of a module that makes this resolution
			// possible.
			var pkg struct {
				ImportPath string
				Export     string
				Module     struct {
					Version string
				}
			}
			var stdout, stderr bytes.Buffer
			list := exec.Command("go", "list", "-export", "-json", g.PreStep.Package)
			list.Stdout = &stdout
			list.Stderr = &stderr
			err := list.Run()
			check(err, "failed to try and list %v: %v\n%s", g.PreStep.Package, err, stderr.Bytes())
			// There should be a single package resolved. If there is more than one
			// this next step will fail... which is what we want.
			err = json.Unmarshal(stdout.Bytes(), &pkg)
			check(err, "failed to decode package: %v; input was\n%s", err, stdout.Bytes())

			if pkg.ImportPath != g.PreStep.Package {
				raise("%v resolved to %v; it should be steady", g.PreStep.Package, pkg.ImportPath)
			}

			if pkg.Module.Version != "" {
				g.PreStep.Version = pkg.Module.Version
			} else {
				g.PreStep.Version = "devel"
				buildid := exec.Command("go", "tool", "buildid", pkg.Export)
				out, err := buildid.CombinedOutput()
				check(err, "failed to get buildid of package %v via %v: %v\n%s", pkg.ImportPath, pkg.Export, err, out)
				g.PreStep.buildID = strings.TrimSpace(string(out))
			}
		}

		type posStep struct {
			pos  token.Position
			step step
		}

		toSort := make(map[string][]posStep)

		for i := 0; i < steps.Len(); i++ {
			fi := steps.Field(i)
			name := fi.Name

			stepV := fi.Value

			// TODO: add support for multiple languages. For now we know
			// there will only be "en"
			lang := "en"
			en := stepV.Lookup(lang)

			if en.Pos() == (token.Pos{}) {
				raise("failed to get position information for step %v; did you embed preguide.#Guide?", fi.Name)
			}

			var step step
			switch {
			case en.Equals(r.commandDef.Unify(en)):
				source, _ := en.Lookup("Source").String()
				step, err = commandStepFromString(name, i, source)
				check(err, "failed to parse #Command from step %v: %v", name, err)
			case en.Equals(r.commandFileDef.Unify(en)):
				path, _ := en.Lookup("Path").String()
				if !filepath.IsAbs(path) {
					path = filepath.Join(g.dir, path)
				}
				step, err = commandStepFromFile(name, i, path)
				check(err, "failed to parse #CommandFile from step %v: %v", name, err)
			case en.Equals(r.uploadDef.Unify(en)):
				target, _ := en.Lookup("Target").String()
				source, _ := en.Lookup("Source").String()
				step = uploadStepFromSource(name, i, source, target)
			case en.Equals(r.uploadFileDef.Unify(en)):
				target, _ := en.Lookup("Target").String()
				path, _ := en.Lookup("Path").String()
				if !filepath.IsAbs(path) {
					path = filepath.Join(g.dir, path)
				}
				step, err = uploadStepFromFile(name, i, path, target)
				check(err, "failed to parse #UploadFile from step %v: %v", name, err)
			default:
				panic(fmt.Errorf("unknown type of step: %v", en))
			}
			toSort[lang] = append(toSort[lang], posStep{
				// TODO: when the FieldInfo position is accurate use that
				pos:  en.Pos().Position(),
				step: step,
			})
		}
		for lang := range toSort {
			g.langs = append(g.langs, lang)
		}
		sort.Strings(g.langs)
		for _, lang := range g.langs {
			steps := toSort[lang]
			sort.Slice(steps, func(i, j int) bool {
				lhs, rhs := steps[i], steps[j]
				cmp := strings.Compare(lhs.pos.Filename, rhs.pos.Filename)
				if cmp == 0 {
					cmp = lhs.pos.Offset - rhs.pos.Offset
				}
				return cmp < 0
			})
			ls := &langSteps{
				Steps: make(map[string]step),
			}
			for _, v := range steps {
				ls.steps = append(ls.steps, v.step)
				ls.Steps[v.step.name()] = v.step
			}
			g.Langs[lang] = ls
		}
	}
}

func (r *runner) loadMarkdownFiles(g *guide) {
	es, err := ioutil.ReadDir(g.dir)
	check(err, "failed to read directory %v: %v", g.dir, err)

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
		if !languageCodes[lang] {
			raise("unknown language code (%q) for %v", lang, path)
		}
		g.mdFiles = append(g.mdFiles, g.buildMarkdownFile(path, lang, ext))
	}
}

var markdownFile = regexp.MustCompile(`.(md|mkdn?|mdown|markdown)$`)

func isMarkdown(name string) (string, bool) {
	ext := markdownFile.FindString(name)
	return ext, ext != ""
}

type mdFile struct {
	path        string
	content     []byte
	frontMatter map[string]interface{}
	frontFormat string
	lang        string
	ext         string
	directives  []directive
}

type directive interface {
	Key() string
	Pos() int
	End() int
}

type baseDirective struct {
	key string

	// pos is the byte offset of the start of the directive
	pos int

	// end is the byte offset of the end of the directive
	end int
}

func (b *baseDirective) Key() string {
	return b.key
}

func (b *baseDirective) Pos() int {
	return b.pos
}

func (b *baseDirective) End() int {
	return b.end
}

type stepDirective struct {
	baseDirective
}

type refDirective struct {
	baseDirective
	val cue.Value
}

type outrefDirective struct {
	baseDirective
	val cue.Value
}

const (
	stepDirectivePrefix       = "step:"
	refDirectivePrefix        = "ref:"
	outrefDirectivePrefix     = "outref:"
	dockerImageFrontMatterKey = "image"
	langFrontMatterKey        = "lang"
)

func (g *guide) buildMarkdownFile(path, lang, ext string) mdFile {
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
		content:     content,
		frontMatter: front.FrontMatter,
		frontFormat: string(front.FrontMatterFormat),
		// dockerImage: image,
	}
	ch := newChunker(content, "<!--", "-->")
	for {
		ok, err := ch.next()
		if err != nil {
			raise("got an error parsing markdown body: %v", err)
		}
		if !ok {
			break
		}
		pos := ch.pos()
		end := ch.end()
		match := content[pos:end]
		htmldoc, err := html.Parse(bytes.NewReader(match))
		check(err, "failed to parse HTML comment %q: %v", match, err)
		if htmldoc.FirstChild.Type != html.CommentNode {
			continue
		}
		commentStr := htmldoc.FirstChild.Data
		switch {
		case strings.HasPrefix(commentStr, stepDirectivePrefix):
			step := &stepDirective{baseDirective: baseDirective{
				key: strings.TrimSpace(strings.TrimPrefix(commentStr, stepDirectivePrefix)),
				pos: pos,
				end: end,
			}}
			res.directives = append(res.directives, step)
		case strings.HasPrefix(commentStr, refDirectivePrefix):
			ref := &refDirective{baseDirective: baseDirective{
				key: strings.TrimSpace(strings.TrimPrefix(commentStr, refDirectivePrefix)),
				pos: pos,
				end: end,
			}}
			res.directives = append(res.directives, ref)
		case strings.HasPrefix(commentStr, outrefDirectivePrefix):
			outref := &outrefDirective{baseDirective: baseDirective{
				key: strings.TrimSpace(strings.TrimPrefix(commentStr, outrefDirectivePrefix)),
				pos: pos,
				end: end,
			}}
			res.directives = append(res.directives, outref)
		}
	}
	return res
}

type chunker struct {
	b   string
	e   string
	buf []byte
	p   int
	ep  int
	lp  int
}

func newChunker(b []byte, beg, end string) *chunker {
	return &chunker{
		buf: b,
		b:   beg,
		e:   end,
	}
}

func (c *chunker) next() (bool, error) {
	find := func(key string) bool {
		p := bytes.Index(c.buf, []byte(key))
		if p == -1 {
			return false
		}
		c.lp = c.p
		c.p = c.ep + p
		c.ep += p + len(key)
		c.buf = c.buf[p+len(key):]
		return true
	}
	if !find(c.b) {
		return false, nil
	}
	if !find(c.e) {
		return false, fmt.Errorf("failed to find end %q terminator", c.e)
	}
	return true, nil
}

func (c *chunker) pos() int {
	return c.lp
}

func (c *chunker) end() int {
	return c.ep
}
