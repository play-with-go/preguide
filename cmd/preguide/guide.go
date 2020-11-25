// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"text/template/parse"

	"cuelang.org/go/cue"
	"github.com/play-with-go/preguide"
	"github.com/play-with-go/preguide/internal/types"
	"gopkg.in/yaml.v2"
)

type StepType int64

const (
	// TODO: keep this in sync with the CUE definitions
	StepTypeCommand StepType = iota + 1
	StepTypeUpload
)

type guide struct {
	dir    string
	name   string
	target string

	mdFiles []mdFile
	langs   []string

	// Embed guideStructure once we have a solution to cuelang.org/issue/376
	Presteps  []*guidePrestep
	Terminals []*preguide.Terminal
	Scenarios []*preguide.Scenario
	Networks  []string
	Env       []string

	Steps      steps
	bashScript string
	Hash       string
	steps      []step

	instance    *cue.Instance
	outinstance *cue.Instance

	outputGuide *guide

	vars []string

	// varMap holds a mapping from {{.VAR}}-style variable name to value.  When
	// a guide needs to be run the value will be the actual value obtained
	// during execution. When a guide does not need to be run then it will be
	// empty. In the latter case, the map is still used in the phase of writing
	// the guide output markdown because the variable name in {{.VAR}} template
	// blocks is normalised and escaped.
	//
	// This will need to be made per language per scenario when that support is
	// added
	varMap map[string]string

	// delims are the text/template delimiters for guide prose and
	// step variable expansion
	Delims [2]string
}

// TODO drop this when we support multiple terminals
func (g *guide) Image() string {
	if got := len(g.Terminals[0].Scenarios); got != 1 {
		panic(fmt.Errorf("expected just 1 scenario, saw %v", got))
	}
	for _, v := range g.Terminals[0].Scenarios {
		return v.Image
	}
	panic("should not be here")
}

// fileSuffix returns a filename suffix appropriate for guide g.
// When we later come to add support for multiple languages and
// scenarios, this signature and logic will need to change
func (g *guide) fileSuffix() string {
	lang := g.langs[0]
	var scenario string
	if len(g.steps) > 0 {
		scenario = g.Scenarios[0].Name + "_"
	}
	return scenario + lang
}

// updateFromOutput is used to update g from an ouput guide out.
// This is typically used when we have a cache hit. That means,
// the input steps are equivalent, in execution terms, to the
// steps in the output schema.
//
// However. There are parameters on the input steps that do
// not affect execution. e.g. on an upload step, the Renderer
// used. Hence we need to copy across fields that represent
// execution output from the output steps onto the input steps.
func (g *guide) updateFromOutput(out *guide) {
	for sn, ostep := range out.Steps {
		istep := g.Steps[sn]
		istep.setOutputFrom(ostep)
	}
	// Populate the guide's varMap based on the variables that resulted
	// when the script did run. Empty values are fine, we just need
	// the environment variable names.
	for _, ps := range out.Presteps {
		for _, v := range ps.Variables {
			g.varMap[v] = ""
		}
	}
	// Now set the guide's Presteps to be that of the output because
	// we known they are equivalent in terms of inputs at this stage
	// i.e. what presteps will run, the order, the args etc, because
	// this check happened as part of the hash check.
	g.Presteps = out.Presteps

}

// Embed *types.Prestep once we have a solution to cuelang.org/issue/376
type guidePrestep struct {
	Package   string
	Path      string
	Args      interface{}
	Version   string
	Variables []string
}

// writeGuideOutput writes the markdown files of output for a guide
// that result from the combination of the configuration and input
// to a guide.
func (pdc *processDirContext) writeGuideOutput() {
	g := pdc.guide

	// TODO: multi language and scenario support. When we have multiple language
	// support we will be running writeGuideOutput() in the context of a language (and
	// scenario). For now we simply use the guide itself, knowing we will have exactly
	// one language and one scenario if we steps, else we have zero scenarios

	var err error

	postsDir := g.target
	err = os.MkdirAll(postsDir, 0777)
	check(err, "failed to os.MkdirAll %v: %v", postsDir, err)

	renderOpts := renderOptions{
		mode: pdc.fMode,
	}

	for _, md := range g.mdFiles {

		outFilePath := filepath.Join(postsDir, fmt.Sprintf("%v_%v%v", g.name, g.fileSuffix(), md.ext))
		outFile, err := os.Create(outFilePath)
		check(err, "failed to open %v for writing: %v", outFilePath, err)

		// TODO: support all front-matter formats
		switch pdc.fMode {
		case types.ModeJekyll:
			switch md.frontFormat {
			case "yaml":
				fmt.Fprintln(outFile, "---")
				if len(md.frontMatter) > 0 {
					enc := yaml.NewEncoder(outFile)
					err := enc.Encode(md.frontMatter)
					check(err, "failed to encode front matter for %v: %v", outFilePath, err)
				}
				fmt.Fprintln(outFile, "---")
			case "":
			default:
				panic(fmt.Errorf("don't yet support front-matter type of %v", md.frontFormat))
			}
		}

		var buf bytes.Buffer

		if len(md.directives) > 0 {
			// TODO: implement fallback to en for directives
			pos := 0
			for _, d := range md.directives {
				buf.Write(md.content[pos:d.Pos().offset])
				switch d := d.(type) {
				case *stepDirective:
					g.Steps[d.name].render(&buf, renderOpts)
				case *refDirective:
					switch d.val.Kind() {
					case cue.StringKind:
						v, _ := d.val.String()
						buf.WriteString(v)
					}
				case *outrefDirective:
					switch d.val.Kind() {
					case cue.StringKind:
						v, _ := d.val.String()
						buf.WriteString(v)
					}
				default:
					panic(fmt.Errorf("don't yet know how to handle %T directives", d))
				}
				pos = d.End().offset
			}
			buf.Write(md.content[pos:])
		} else {
			buf.Write(md.content)
		}

		switch pdc.fMode {
		case types.ModeJekyll:
			// Now write a simple <script> block that declares some useful variables
			// that will be picked up by postLayout.js
			//
			// TODO: obviously this code needs to change when we run multiple
			// scenarios.
			if len(g.Scenarios) > 0 {
				fmt.Fprintf(&buf, "<script>let pageGuide=%q; let pageLanguage=%q; let pageScenario=%q;</script>\n", g.name, md.lang, g.Scenarios[0].Name)
			}
		}

		// At this stage, random values have been sanitised to deterministic
		// values, and env values have been substituted for {{{.ENV}}}
		// equivalents (using whatever delimeters are configured for the guide).
		// But critically, in command/code blocks, { and } have been replaced with
		// their HTML entity equivalents.
		//
		// Therefore the only remaining step is to replace { and } instances in
		// {{{.ENV}}} templates that appear in the prose. We do this using {% raw
		// %} blocks in Jekyll mode because we can't know whether the user will
		// use such a template within a `` code element, in which case the
		// ampersand in &#123; would be interpreted literally and &#123; would be
		// rendered as  &#123;.
		//
		// If the author wants to include a literal { or } in their markdown
		// input, they can use &#123; or &#125;
		//
		// Script output is assumed to only ever include literal { and } values
		// hence that is unconditionally escaped using &#123; and &#125;
		repls := g.varMap
		if pdc.genCmd.fMode == types.ModeJekyll {
			repls = make(map[string]string)
			for v := range g.varMap {
				repls[v] = "{% raw %}" + g.Delims[0] + "." + v + g.Delims[1] + "{% endraw %}"
			}
		}
		t := template.New("prose {{.ENV}} normalising and escaping")
		pt, err := parse.Parse(t.Name(), buf.String(), g.Delims[0], g.Delims[1])
		check(err, "failed to parse output for prose {{.ENV}} normalising and escaping")
		t.Delims(g.Delims[0], g.Delims[1])
		t.AddParseTree(t.Name(), pt[t.Name()])
		t.Option("missingkey=error")
		err = t.Execute(outFile, repls)
		check(err, "failed to execute prose {{.ENV}} normalising and escaping template: %v", err)

		err = outFile.Close()
		check(err, "failed to close %v: %v", outFilePath, err)
	}
}

// writeLog writes a
func (pdc *processDirContext) writeLog() {
	g := pdc.guide

	// TODO: multi language and scenario support. When we have multiple language
	// support we will be running writeLog() in the context of a language (and
	// scenario). For now we simply use the guide itself, knowing we will have exactly
	// one language and one scenario if we steps, else we have zero scenarios

	var buf bytes.Buffer
	for _, step := range g.steps {
		step.renderLog(pdc.fMode, &buf)
	}
	logFilePath := filepath.Join(g.dir, fmt.Sprintf("%v_log.txt", g.fileSuffix()))
	err := ioutil.WriteFile(logFilePath, buf.Bytes(), 0666)
	check(err, "failed to write log output to %v: %v", logFilePath, err)
}

func mustJSONMarshalIndent(i interface{}) []byte {
	byts, err := json.MarshalIndent(i, "", "  ")
	check(err, "failed to marshal prestep: %v", err)
	return byts

}
