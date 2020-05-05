// preguide is a pre-processor for Play With Docker-based guides
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
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
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

type runner struct {
	flagSet *flag.FlagSet

	// fDir is the directory in which to run preguide
	fDir *string

	// fOutput is the directory into which preguide should place
	// generated content
	fOutput *string

	fDebug         *bool
	fSkipCache     *bool
	fImageOverride *string
	fCompat        *bool

	runtime cue.Runtime

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

func (r *runner) mainerr() (err error) {
	defer handleKnown(&err)

	r.codec = gocodec.New(&r.runtime, nil)

	if err := r.flagSet.Parse(os.Args[1:]); err != nil {
		return flagErr(err.Error())
	}
	if r.fOutput == nil || *r.fOutput == "" {
		return usageErr("target directory must be specified")
	}

	r.loadSchemas()

	dir, err := filepath.Abs(*r.fDir)
	check(err, "failed to make path %q absolute: %v", *r.fDir, err)

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
		target: *r.fOutput,
		Langs:  make(map[string]*langSteps),
	}

	r.loadMarkdownFiles(g)
	if len(g.mdFiles) == 0 {
		return
	}

	r.loadSteps(g)
	r.loadOutput(g)

	// TODO: support arbitrary top-level fields (other than those required to
	// satisfy #GuideOutput) to act as directive keys. This will in effect
	// require building a set of input step and the output top-level references.

	// TODO: verify that we have identical sets of languages when we support
	// multiple languages

	dirCount := 0
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
			dirCount++
			switch d := d.(type) {
			case *stepDirective:
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
			default:
				panic(fmt.Errorf("don't yet know how to handle %T type", d))
			}
		}
	}

	stepCount := 0
	for _, ls := range g.Langs {
		stepCount += len(ls.steps)
	}

	if dirCount == 0 && stepCount > 0 {
		fmt.Fprintln(os.Stderr, "This guide does not have any directives but does have steps to run.")
		fmt.Fprintln(os.Stderr, "Not running those steps because they are not referenced.")
	}

	if stepCount > 0 {
		for _, l := range g.langs {
			ls := g.Langs[l]
			r.buildBashFile(g, ls)
			if !*r.fSkipCache {
				if out := g.outputGuide; out != nil {
					if ols := out.Langs[l]; ls != nil {
						if ols.Hash == ls.Hash {
							r.debugf("cache hit for %v: will not re-run script\n", l)
							ls.Steps = ols.Steps
							ls.steps = ols.steps
							continue
						}
					}
				}
			}
			r.runBashFile(g, ls)
		}
		r.writeOutput(g)
	}

	r.process(g)
	r.generateTestLog(g)
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

func (r *runner) loadOutput(g *guide) {
	conf := &load.Config{
		Dir: g.dir,
	}
	bps := load.Instances([]string{"./out"}, conf)
	gp := bps[0]
	if gp.Err != nil {
		// absorb this error - we have nothing to do. We will fix
		// out when we write the output later (all being well)
		return
	}

	gi, err := r.runtime.Build(gp)
	check(err, "failed to build %v: %v", gp.ImportPath, err)

	// gv is the value that represents the guide's CUE package
	gv := gi.Value()

	if gv.Unify(r.guideOutDef).Validate() != nil {
		return
	}

	var out guide
	r.encodeGuide(gv, &out)

	g.outputGuide = &out
	g.output = gv
}

func (r *runner) runBashFile(g *guide, ls *langSteps) {
	td, err := ioutil.TempDir("", fmt.Sprintf("preguide-%v-runner-", g.name))
	check(err, "failed to create temp directory for guide %v: %v", g.dir, err)
	defer os.RemoveAll(td)
	sf := filepath.Join(td, "script.sh")
	err = ioutil.WriteFile(sf, []byte(ls.bashScript), 0777)
	check(err, "failed to write temporary script to %v: %v", sf, err)

	cmd := exec.Command("docker", "run", "--rm", "-v", fmt.Sprintf("%v:/scripts", td), g.Image, "/scripts/script.sh")
	out, err := cmd.CombinedOutput()
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
				if stmt.sanitiser != nil {
					stmt.Output = stmt.sanitiser(stmt.Output)
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

	steps, err := gv.Lookup("Steps").Struct()

	g.Image, _ = gv.Lookup("Image").String()
	if *r.fImageOverride != "" {
		g.Image = *r.fImageOverride
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

const (
	stepDirectivePrefix       = "step:"
	refDirectivePrefix        = "ref:"
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
	parser := goldmark.DefaultParser()
	reader := text.NewReader(content)
	doc := parser.Parse(reader)
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var pos, end int
		switch n := n.(type) {
		case *ast.HTMLBlock:
			firstSeg := n.Lines().At(0)
			pos = firstSeg.Start
			if cl := n.ClosureLine; cl.Start != -1 && cl.Stop != -1 {
				end = cl.Stop
			} else {
				end = firstSeg.Stop
			}
		case *ast.RawHTML:
			pos = n.Segments.At(0).Start
			end = n.Segments.At(n.Segments.Len() - 1).Stop
		default:
			return ast.WalkContinue, nil
		}
		htmlComment := content[pos:end]
		htmldoc, err := html.Parse(bytes.NewReader(htmlComment))
		check(err, "failed to parse HTML %q: %v", htmlComment, err)
		if htmldoc.FirstChild.Type != html.CommentNode {
			return ast.WalkContinue, nil
		}
		commentStr := htmldoc.FirstChild.Data
		switch {
		case strings.HasPrefix(commentStr, stepDirectivePrefix):
			step := &stepDirective{}
			step.key = strings.TrimSpace(strings.TrimPrefix(commentStr, stepDirectivePrefix))
			step.pos = pos
			step.end = end
			res.directives = append(res.directives, step)
		case strings.HasPrefix(commentStr, refDirectivePrefix):
			ref := &refDirective{}
			ref.key = strings.TrimSpace(strings.TrimPrefix(commentStr, refDirectivePrefix))
			ref.pos = pos
			ref.end = end
			res.directives = append(res.directives, ref)
		default:
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	return res
}
