package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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
	langs   []types.LangCode

	// Embed guideStructure once we have a solution to cuelang.org/issue/376
	Presteps  []*guidePrestep
	Terminals []*preguide.Terminal
	Scenarios []*preguide.Scenario
	Networks  []string
	Env       []string

	Langs map[types.LangCode]*langSteps

	instance    *cue.Instance
	outinstance *cue.Instance

	outputGuide *guide
	output      cue.Value

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
func (r *runner) writeGuideOutput(g *guide) {
	if len(g.mdFiles) != 1 || g.mdFiles[0].lang != "en" {
		raise("we only support English language guides for now")
	}

	var err error

	postsDir := filepath.Join(g.target, "_posts")
	err = os.MkdirAll(postsDir, 0777)
	check(err, "failed to os.MkdirAll %v: %v", postsDir, err)

	for _, md := range g.mdFiles {
		// TODO: multi-language support

		outFilePath := filepath.Join(postsDir, fmt.Sprintf("%v%v", g.name, md.ext))
		outFile, err := os.Create(outFilePath)
		check(err, "failed to open %v for writing: %v", outFilePath, err)

		// TODO: support all front-matter formats
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

		var buf bytes.Buffer

		if len(md.directives) > 0 {
			// TODO: implement fallback to en for directives
			var steps map[string]step
			if ls := g.Langs[md.lang]; ls != nil {
				steps = ls.Steps
			}
			pos := 0
			for _, d := range md.directives {
				buf.Write(md.content[pos:d.Pos()])
				switch d := d.(type) {
				case *stepDirective:
					if *r.genCmd.fCompat {
						steps[d.Key()].renderCompat(&buf)
					} else {
						steps[d.Key()].render(&buf)
					}
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
				pos = d.End()
			}
			buf.Write(md.content[pos:])
		} else {
			buf.Write(md.content)
		}

		// Now write a simple <script> block that declares some useful variables
		// that will be picked up by postLayout.js
		//
		// TODO: obviously this code needs to change when we run multiple
		// scenarios.
		if len(g.Scenarios) > 0 {
			fmt.Fprintf(&buf, "<script>let pageGuide=%q; let pageLanguage=%q; let pageScenario=%q;</script>\n", g.name, md.lang, g.Scenarios[0].Name)
		}

		// If we are in normal (non-raw) mode, then we want to substitute
		// {{.ENV}} templates with {% raw %}{{.ENV}}{% endraw %} normalised
		// templates. Note this step is necessary here because the command and
		// file inputs that contain {{.ENV}} templates are, at this stage,
		// untouched. They get replaced as part of running the script but not as
		// part of the writing of the output markdown file. The output
		// sanitisation handles the replacing of env var values with their
		// variable names, this step does the overall normalisation (and
		// escaping) of _all_ {{.ENV}} templates.
		//
		// If we are in raw mode then we want to substitute {{.ENV}} templates
		// for their actual value.
		//
		// TODO: it seems less than ideal that we are performing this substitution
		// post directive replacement. Far better would be that we perform it
		// pre directive replacement. However, that would require us to parse
		// markdown files twice: the first time to establish the list of directives
		// present, the second time post the substitution of {{.ENV}} templates.
		// It's not entirely clear what is more correct here. However, it doesn't
		// really matter because this only affects raw mode, which is essentially a
		// debug-only mode for now.
		//
		// However, if there are no vars, then the substitution will have zero
		// effect (regardless of whether there are any templates to be expanded)
		if !*r.genCmd.fRaw || len(g.vars) == 0 {
			// Build a map of the variable names to escape
			escVarMap := make(map[string]string)
			for v := range g.varMap {
				escVarMap[v] = "{% raw %}{{." + v + "}}{% endraw %}"
			}
			t := template.New("{{.ENV}} normalising and escaping")
			t.Option("missingkey=error")
			_, err := t.Parse(buf.String())
			check(err, "failed to parse output for {{.ENV}} normalising and escaping")
			err = t.Execute(outFile, escVarMap)
			check(err, "failed to execute {{.ENV}} normalising and escaping template: %v", err)
		} else {
			t := template.New("pre-substitution markdown")
			t.Option("missingkey=error")
			_, err = t.Parse(buf.String())
			check(err, "failed to parse pre-substitution markdown: %v", err)
			err = t.Execute(outFile, g.varMap)
			check(err, "failed to execute pre-substitution markdown template: %v", err)
		}

		err = outFile.Close()
		check(err, "failed to close %v: %v", outFilePath, err)
	}
}

// writeLog writes a
func (r *runner) writeLog(g *guide) {
	for lang, ls := range g.Langs {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Terminals: %s\n", mustJSONMarshalIndent(g.Terminals))
		if len(g.Presteps) > 0 {
			fmt.Fprintf(&buf, "Presteps: %s\n", mustJSONMarshalIndent(g.Presteps))
		}
		for _, step := range ls.steps {
			step.renderLog(&buf)
		}
		logFilePath := filepath.Join(g.dir, fmt.Sprintf("%v_log.txt", lang))
		err := ioutil.WriteFile(logFilePath, buf.Bytes(), 0666)
		check(err, "failed to write log output to %v: %v", logFilePath, err)
	}
}

func mustJSONMarshalIndent(i interface{}) []byte {
	byts, err := json.MarshalIndent(i, "", "  ")
	check(err, "failed to marshal prestep: %v", err)
	return byts

}

func (g *guide) sanitiseVars(s string) string {
	for name, val := range g.varMap {
		s = strings.ReplaceAll(s, val, "{{."+name+"}}")
	}
	return s
}
