// preguide is a pre-processor for Play With Docker-based guides
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/gocode/gocodec"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/kr/pretty"
	"github.com/play-with-go/preguide/internal/embed"
	"github.com/play-with-go/preguide/internal/types"
	"golang.org/x/net/html"
)

type runner struct {
	*rootCmd
	genCmd    *genCmd
	initCmd   *initCmd
	helpCmd   *helpCmd
	dockerCmd *dockerCmd

	runtime cue.Runtime

	dir string

	preguideBuildInfo string

	codec *gocodec.Codec

	guideDef       cue.Value
	commandDef     cue.Value
	commandFileDef cue.Value
	uploadDef      cue.Value
	uploadFileDef  cue.Value
	guideOutDef    cue.Value
	commandStep    cue.Value
	uploadStep     cue.Value

	// seenPrestepPkgs is a cache of the presteps we have seen and resolved
	// to a version in a given run of preguide
	seenPrestepPkgs map[string]string
}

func newRunner() *runner {
	return &runner{
		dir:             ".",
		seenPrestepPkgs: make(map[string]string),
	}
}

func (r *runner) mainerr() (err error) {
	defer handleKnown(&err)

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		raise("failed to read build info")
	}
	if bi.Main.Replace != nil {
		bi.Main = *bi.Main.Replace
	}
	if bi.Main.Sum == "" {
		// Local development. Use the export information if it is available
		export := exec.Command("go", "list", "-mod=readonly", "-export", "-f={{.Export}}", "github.com/play-with-go/preguide")
		out, err := export.CombinedOutput()
		if err == nil {
			r.preguideBuildInfo = string(out)
		} else {
			// The only really conceivable case where this should happen is development
			// of preguide itself. In that case, we will be running testscript tests
			// that start from a clean slate.
			r.preguideBuildInfo = "devel"
		}
	} else {
		r.preguideBuildInfo = bi.Main.Version + " " + bi.Main.Sum
	}

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
	case "docker":
		return r.runDocker(args[1:])
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

func (r *runner) runDocker(args []string) error {
	// Usage:
	//
	//     preguide docker METHOD URL ARGS
	//
	// where ARGS is a JSON-encoded string. Returns (via stdout) the JSON-encoded result
	// (without checking that result)

	var body io.Reader

	switch len(args) {
	case 2:
	case 3:
		body = strings.NewReader(args[2])
	default:
		return r.dockerCmd.usageErr("expected either 2 or 3 args; got %v", len(args))
	}

	method, url := args[0], args[1]

	req, err := http.NewRequest(method, url, body)
	check(err, "failed to build a new request for a %v to %v: %v", method, url, err)

	resp, err := http.DefaultClient.Do(req)
	check(err, "failed to execute %v: %v", req, err)

	_, err = io.Copy(os.Stdout, resp.Body)
	check(err, "failed to read response body from %v: %v", req, err)

	return nil
}

func (r *runner) runInit(args []string) error {
	return nil
}

func (r *runner) runGen(args []string) error {
	if err := r.genCmd.fs.Parse(args); err != nil {
		return r.genCmd.usageErr("failed to parse flags: %v", err)
	}
	if r.genCmd.fOutput == nil || *r.genCmd.fOutput == "" {
		return r.genCmd.usageErr("target directory must be specified")
	}

	// Fallback to env-supplied config if no values supplied via -config flag
	if len(r.genCmd.fConfigs) == 0 {
		envVals := strings.Split(os.Getenv("PREGUIDE_CONFIG"), ":")
		for _, v := range envVals {
			v = strings.TrimSpace(v)
			if v != "" {
				r.genCmd.fConfigs = append(r.genCmd.fConfigs, v)
			}
		}
	}

	r.genCmd.loadConfig()

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

// genConfig defines a mapping between the prestep pkg (which is essentially the
// unique identifier for a prestep) and config for that prestep. For example,
// github.com/play-with-go/gitea will map to an endpoint that explains where that
// prestep can be "found". The Networks value represents a (non-production) config
// that describes which Docker networks the request should be made within.
type genConfig map[string]*prestepConfig

type prestepConfig struct {
	Endpoint *url.URL
	Networks []string
}

func (p *prestepConfig) UnmarshalJSON(b []byte) error {
	var v struct {
		Endpoint string
		Networks []string
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("failed to unmarshal prestepConfig: %v", err)
	}
	u, err := url.Parse(v.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse URL from prestepConfig Endpoint %q: %v", v.Endpoint, err)
	}
	p.Endpoint = u
	p.Networks = v.Networks
	return nil
}

func (g *genCmd) loadConfig() {
	if len(g.fConfigs) == 0 {
		return
	}

	// TODO two things need to improve here:
	//
	// 1. We should code generate a CUE version of the genConfig type
	// so that people can use that schema as they wish. This is essentially
	// a request for the missing half of cuelang.org/go/encoding/gocode
	// 2. At runtime when validating -config inputs we should extract the
	// definition via something like cuelang.org/go/encoding/gocode/gocodec
	// and the ExtractType method. However at the moment this does not support
	// extracting a definition directly, neither does the cuelang.org/go/cue
	// API support deriving a closed value of a *cue.Struct
	//
	// So for now we maintain the genConfig type and the following string const
	// of CUE code by hand
	var r cue.Runtime
	const schemaDef = `
	#def: [string]: {
		Endpoint: string
		Networks: [...string]
	}
	`
	schemaInst, err := r.Compile("schema.cue", schemaDef)
	check(err, "failed to compile config schema: %v", err)
	check(schemaInst.Err, "failed to load config schema: %v", schemaInst.Err)

	schema := schemaInst.LookupDef("def")
	err = schema.Err()
	check(err, "failed to lookup schema definition: %v", err)

	// res will hold the config result
	var res cue.Value

	bis := load.Instances(g.fConfigs, nil)
	for i, bi := range bis {
		inst, err := r.Build(bi)
		check(err, "failed to load config from %v: %v", g.fConfigs[i], err)
		res = res.Unify(inst.Value())
	}

	res = schema.Unify(res)
	err = res.Validate()
	check(err, "failed to validate input config: %v", err)

	// Now we can extract the config from the CUE
	codec := gocodec.New(&r, nil)
	err = codec.Encode(res, &g.config)
	check(err, "failed to decode config from CUE value: %v", err)

	// Now validate that we don't have any networks for file protocol endpoints
	for ps, conf := range g.config {
		if conf.Endpoint.Scheme == "file" && len(conf.Networks) > 0 {
			raise("prestep %v defined a file scheme endpoint %v but provided networks [%v]", ps, conf.Endpoint, conf.Networks)
		}
	}
}

func (r *runner) debugf(format string, args ...interface{}) {
	if *r.fDebug {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func (r *runner) loadSchemas() {
	overlay := make(map[string]load.Source)
	for _, asset := range embed.AssetNames() {
		contents, err := embed.Asset(asset)
		if err != nil {
			panic(err)
		}
		overlay[filepath.Join("/", asset)] = load.FromBytes(contents)
	}
	conf := &load.Config{
		Dir:     "/",
		Overlay: overlay,
	}
	bps := load.Instances([]string{".", "./out"}, conf)
	preguide, err := r.runtime.Build(bps[0])
	check(err, "failed to compile github.com/play-with-go/preguide package: %v", err)
	preguideOut, err := r.runtime.Build(bps[1])
	check(err, "failed to compile github.com/play-with-go/preguide/out package: %v", err)

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
		Langs:  make(map[types.LangCode]*langSteps),
		varMap: make(map[string]string),
	}

	r.loadMarkdownFiles(g)
	if len(g.mdFiles) == 0 {
		return
	}

	r.validateAndLoadsSteps(g)
	if !*r.genCmd.fRaw {
		r.loadOutput(g, false)
	}

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
		if !*r.genCmd.fRaw && (outputLoadRequired || g.outputGuide == nil) {
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
		mdf.frontMatter[guideFrontMatterKey] = g.name

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
	enc := gocodec.New(&r.runtime, nil)
	v, err := enc.Decode(g)
	check(err, "failed to decode guide to CUE: %v", err)
	byts, err := format.Node(v.Syntax())
	out := fmt.Sprintf("package out\n\n%s\n", byts)
	check(err, "failed to format CUE output: %v", err)

	// If we are in raw mode we dump output to stdout. It's more of a debugging mode
	if *r.genCmd.fRaw {
		fmt.Printf("%s", out)
		return
	}

	outDir := filepath.Join(g.dir, "out")
	err = os.MkdirAll(outDir, 0777)
	check(err, "failed to mkdir %v: %v", outDir, err)
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
	err = gv.Decode(&out)
	check(err, "failed to decode Guide from out value: %v", err)

	// Now populate the steps slice for each langSteps
	for _, ls := range out.Langs {
		for _, step := range ls.Steps {
			ls.steps = append(ls.steps, step)
		}
		sort.Slice(ls.steps, func(i, j int) bool {
			return ls.steps[i].order() < ls.steps[j].order()
		})
	}

	g.outputGuide = &out
	g.output = gv
	g.outinstance = gi
}

func (r *runner) runBashFile(g *guide, ls *langSteps) {
	// Now run the pre-step if there is one
	var toWrite string
	for _, ps := range g.Presteps {
		// TODO: run the presteps concurrently, but add their args in order
		// last prestep's args last etc

		var jsonBody []byte

		// At this stage we know we have a valid endpoint (because we previously
		// checked it via a get-version=1 request)
		conf := r.genCmd.config[ps.Package]
		if conf.Endpoint.Scheme == "file" {
			if len(ps.Args) > 0 {
				raise("prestep %v provides with arguments [%v]: but prestep is configured with a file endpoint", ps.Package, pretty.Sprint(ps.Args))
			}
			// Notice this path takes no account of the -docker flag
			var err error
			path := conf.Endpoint.Path
			jsonBody, err = ioutil.ReadFile(path)
			check(err, "failed to read file endpoint %v (file %v): %v", conf.Endpoint, path, err)
		} else {
			jsonBody = r.genCmd.doRequest("POST", conf.Endpoint.String(), conf.Networks, ps.Args) // Do not splat args
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
		}
	}
	// If we have any vars we need to first perform an expansion of any
	// templates instances {{.ENV}} that appear in the bashScript, and then
	// append the result of that substitution
	bashScript := ls.bashScript
	if len(g.vars) > 0 {
		t := template.New("pre-substitution bashScript")
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
	td, err := ioutil.TempDir("", fmt.Sprintf("preguide-%v-runner-", g.name))
	check(err, "failed to create temp directory for guide %v: %v", g.dir, err)
	defer os.RemoveAll(td)
	sf := filepath.Join(td, "script.sh")
	err = ioutil.WriteFile(sf, []byte(toWrite), 0777)
	check(err, "failed to write temporary script to %v: %v", sf, err)

	// Whilst we know we have a single terminal, we can use the g.Image() hack
	// of finding the image for that single terminal. We we support multiple
	// terminals we will need to move away from that hack
	image := g.Image()
	if *r.genCmd.fImageOverride != "" {
		image = *r.genCmd.fImageOverride
	}

	imageCheck := exec.Command("docker", "inspect", image)
	out, err := imageCheck.CombinedOutput()
	if err != nil {
		if *r.genCmd.fPullImage == pullImageMissing {
			r.debugf("failed to find docker image %v (%v); will attempt pull", image, err)
			pull := exec.Command("docker", "pull", image)
			out, err = pull.CombinedOutput()
			check(err, "failed to find docker image %v; also failed to pull it: %v\n%s", image, err, out)
		} else {
			raise("failed to find docker image %v (%v); either pull this image manually or use -pull=missing", image, err)
		}
	}

	cmd := exec.Command("docker", "run", "--rm",
		"-v", fmt.Sprintf("%v:/scripts", td),
		"-e", fmt.Sprintf("USER_UID=%v", os.Geteuid()),
		"-e", fmt.Sprintf("USER_GID=%v", os.Getegid()),
	)
	for _, v := range g.vars {
		cmd.Args = append(cmd.Args, "-e", v)
	}
	cmd.Args = append(cmd.Args, image, "/scripts/script.sh")
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
				stmt.Output = slurp(fence)
				if !*r.genCmd.fRaw {
					for _, s := range append(stmt.sanitisers, g.sanitiseVars) {
						stmt.Output = s(stmt.Output)
					}
				}
				exitCodeStr := slurp([]byte("\n"))
				stmt.ExitCode, err = strconv.Atoi(exitCodeStr)
				check(err, "failed to parse exit code from %q at position %v in output: %v\n%s", exitCodeStr, len(out)-len(walk)-len(exitCodeStr)-1, err, out)
			}
		}
	}
}

func (r *runner) buildBashFile(g *guide, ls *langSteps) {
	// TODO when we come to support multiple terminals this will need to be
	// rethought. Perhaps something along the following lines:
	//
	// * Create a special bash script per terminal
	//
	// * The order of steps is defined by the natural source order of step
	// names. That is to say, the first time a step declaration is encountered
	// determines that step's position in the order of all steps
	//
	// * The control flow between terminals is managed by blocking bash. This
	// seems doable with a read call, and a kill -SIGINT to the bash process
	// then interrupts that
	//
	// * We only need to block a given terminal at the "edge" of handover
	// between terminals
	//
	// * We can determine the process ID of the special bash script by echo-ing
	// $$ at the start of the special script.
	//
	// * This means we will need to attach stdout, stderr and stdin to each of
	// the docker run instances for each terminal. This is fine because it
	// opens the door for us being able to supply input over stdin should this
	// ever be necessary
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

	var sb strings.Builder
	pf := func(format string, args ...interface{}) {
		fmt.Fprintf(&sb, format, args...)
	}
	h := sha256.New()
	hf := func(format string, args ...interface{}) {
		fmt.Fprintf(h, format, args...)
	}
	// Write the module info for github.com/play-with-go/preguide
	hf("preguide: %#v\n", r.preguideBuildInfo)
	// We write the Presteps information to the hash, and only run the pre-step
	// if we have a cache miss and come to run the bash file. Note that
	// this _includes_ the buildID (hence the use of pretty.Sprint rather
	// than JSON), whereas in the testlog we use JSON to _not_ include the
	// buildID
	hf("prestep: %s\n", pretty.Sprint(g.Presteps))
	// We write the docker image to the hash, because if the user want to ensure
	// reproducibility they should specify the full digest.
	hf("image: %v\n", g.Image())
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

func (r *runner) validateAndLoadsSteps(g *guide) {
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
		check(gp.Err, "failed to load CUE package in %v: %v", g.dir, gp.Err)
	}

	gi, err := r.runtime.Build(gp)
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
	gv = gv.Unify(r.guideDef)
	err = gv.Validate()
	check(err, "%v does not satisfy github.com/play-with-go/preguide.#Guide: %v", gp.ImportPath, err)

	var intGuide types.Guide
	err = gv.Decode(&intGuide)
	check(err, "failed to decode guide: %v", err)

	g.delims = intGuide.Delims

	type termPosition struct {
		name string
		pos  token.Pos
	}
	var termPositions []termPosition
	if len(intGuide.Terminals) > 1 {
		raise("we don't currently support multiple terminals")
	}
	for n := range intGuide.Terminals {
		termPositions = append(termPositions, termPosition{
			name: n,
			pos:  structPos(gv.Lookup("Terminals", n)),
		})
	}
	sort.Slice(termPositions, func(i, j int) bool {
		return posLessThan(termPositions[i].pos, termPositions[j].pos)
	})
	for _, tp := range termPositions {
		n := tp.name
		t := intGuide.Terminals[n]
		g.Terminals = append(g.Terminals, newTerminal(n, t))
	}

	if len(intGuide.Steps) > 0 {
		// We only investigate the presteps if we have any steps
		// to run
		for _, prestep := range intGuide.Presteps {
			ps := guidePrestep{
				Package: prestep.Package,
				Args:    prestep.Args,
			}
			if ps.Package == "" {
				raise("Prestep had empty package")
			}
			if v, ok := r.seenPrestepPkgs[ps.Package]; ok {
				ps.Version = v
			} else {
				// Resolve and endpoint for the package
				conf, ok := r.genCmd.config[ps.Package]
				if !ok {
					raise("no config found for prestep %v", ps.Package)
				}
				var version string
				if conf.Endpoint.Scheme == "file" {
					version = "file"
				} else {
					version = string(r.genCmd.doRequest("GET", conf.Endpoint.String()+"?get-version=1", conf.Networks))
				}
				r.seenPrestepPkgs[ps.Package] = version
				ps.Version = version
			}
			g.Presteps = append(g.Presteps, &ps)
		}
	}

	seenLangs := make(map[types.LangCode]bool)

	for stepName, langSteps := range intGuide.Steps {

		for _, code := range types.Langs {
			v, ok := langSteps[code]
			if !ok {
				continue
			}
			seenLangs[code] = true
			var s step
			switch is := v.(type) {
			case *types.Command:
				s, err = commandStepFromCommand(stepName, is)
				check(err, "failed to parse #Command from step %v: %v", stepName, err)
			case *types.CommandFile:
				if !filepath.IsAbs(is.Path) {
					is.Path = filepath.Join(g.dir, is.Path)
				}
				s, err = commandStepFromCommandFile(stepName, is)
				check(err, "failed to parse #CommandFile from step %v: %v", stepName, err)
			case *types.Upload:
				s, err = uploadStepFromUpload(stepName, is)
				check(err, "failed to parse #Upload from step %v: %v", stepName, err)
			case *types.UploadFile:
				if !filepath.IsAbs(is.Path) {
					is.Path = filepath.Join(g.dir, is.Path)
				}
				s, err = uploadStepFromUploadFile(stepName, is)
				check(err, "failed to parse #UploadFile from step %v: %v", stepName, err)
			}
			ls, ok := g.Langs[code]
			if !ok {
				ls = newLangSteps()
				g.Langs[code] = ls
			}
			ls.Steps[stepName] = s
		}
	}

	for code := range seenLangs {
		g.langs = append(g.langs, code)
	}
	sort.Slice(g.langs, func(i, j int) bool {
		return g.langs[i] < g.langs[j]
	})

	// Sort according to the order of the steps as declared in the
	// guide [filename, offset]
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
	sort.Slice(stepPositions, func(i, j int) bool {
		return posLessThan(stepPositions[i].pos, stepPositions[j].pos)
	})
	for _, code := range types.Langs {
		ls, ok := g.Langs[code]
		if !ok {
			continue
		}
		for i, sp := range stepPositions {
			s, ok := ls.Steps[sp.name]
			if !ok {
				raise("lang %v does not define step %v; we don't yet support fallback logic", code, sp.name)
			}
			ls.steps = append(ls.steps, s)
			s.setorder(i)
		}
	}
}

// structPos returns the position of the struct value v. This helper
// is required because of cuelang.org/issues/480
func structPos(v cue.Value) token.Pos {
	if v.Err() != nil {
		raise("asked to find struct position of error value: %v", v.Err())
	}
	pos := v.Pos()
	if pos == (token.Pos{}) {
		it, err := v.Fields()
		if err != nil {
			return token.Pos{}
		}
		for it.Next() {
			fp := structPos(it.Value())
			if posLessThan(fp, pos) {
				pos = fp
			}
		}
	}
	return pos
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
func (g *genCmd) doRequest(method string, endpoint string, networks []string, args ...interface{}) []byte {
	var body io.Reader
	if len(args) > 0 {
		byts, err := json.Marshal(args[0])
		check(err, "failed to encode args: %v", err)
		body = bytes.NewReader(byts)
	}
	if *g.fDocker != "" {
		sself, err := os.Executable()
		check(err, "failed to derive executable for self: %v", err)
		self, err := filepath.EvalSymlinks(sself)
		check(err, "failed to eval symlinks for %v: %v", sself, err)

		createCmd := exec.Command("docker", "create",

			// Don't leave this container around
			"--rm",

			// Set up "ourselves" as the entrypoint.
			fmt.Sprintf("--volume=%s:/init", self),
			"--entrypoint=/init",
		)

		if v, ok := os.LookupEnv("TESTSCRIPT_COMMAND"); ok {
			createCmd.Args = append(createCmd.Args, "-e", "TESTSCRIPT_COMMAND="+v)
		}

		// Add the user-supplied args, after splitting docker flag val into
		// pieces
		addArgs, err := split(*g.fDocker)
		check(err, "failed to split -docker flag into args: %v", err)
		createCmd.Args = append(createCmd.Args, addArgs...)

		// Now add the arguments to "ourselves"
		createCmd.Args = append(createCmd.Args, "docker", method, endpoint)
		if body != nil {
			byts, err := ioutil.ReadAll(body)
			check(err, "failed to read from body: %v", err)
			createCmd.Args = append(createCmd.Args, string(byts))
		}

		var createStdout, createStderr bytes.Buffer
		createCmd.Stdout = &createStdout
		createCmd.Stderr = &createStderr

		err = createCmd.Run()
		check(err, "failed to run [%v]: %v\n%s", strings.Join(createCmd.Args, " "), err, createStderr.Bytes())

		instance := strings.TrimSpace(createStdout.String())

		if len(networks) > 0 {
			for _, network := range networks {
				connectCmd := exec.Command("docker", "network", "connect", network, instance)
				out, err := connectCmd.CombinedOutput()
				check(err, "failed to run [%v]: %v\n%s", strings.Join(connectCmd.Args, " "), err, out)
			}
		}
		startCmd := exec.Command("docker", "start", "-a", instance)
		var startStdout, startStderr bytes.Buffer
		startCmd.Stdout = &startStdout
		startCmd.Stderr = &startStderr

		err = startCmd.Run()
		check(err, "failed to run [%v]: %v\n%s", strings.Join(startCmd.Args, " "), err, startStderr.Bytes())

		return startStdout.Bytes()
	}

	req, err := http.NewRequest(method, endpoint, body)
	check(err, "failed to build HTTP request for method %v, url %q: %v", method, endpoint, err)
	resp, err := http.DefaultClient.Do(req)
	check(err, "failed to perform HTTP request with args [%v]: %v", args, err)
	respBody, err := ioutil.ReadAll(resp.Body)
	check(err, "failed to read response body for request %v: %v", req, err)
	return respBody
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
		if !types.ValidLangCode(lang) {
			raise("unknown language code (%q) for %v", lang, path)
		}
		g.mdFiles = append(g.mdFiles, g.buildMarkdownFile(path, types.LangCode(lang), ext))
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
	lang        types.LangCode
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
	guideFrontMatterKey       = "guide"
	langFrontMatterKey        = "lang"
)

func (g *guide) buildMarkdownFile(path string, lang types.LangCode, ext string) mdFile {
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
