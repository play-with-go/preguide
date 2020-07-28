package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"gopkg.in/yaml.v2"
)

type guide struct {
	dir    string
	name   string
	target string

	mdFiles []mdFile
	langs   []string

	Presteps []*guidePrestep

	Image string
	Langs map[string]*langSteps

	instance    *cue.Instance
	outinstance *cue.Instance

	outputGuide *guide
	output      cue.Value

	vars []string
}

type guidePrestep struct {
	Package string
	Version string
	buildID string
	Args    []string
}

func (r *runner) process(g *guide) {
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

		_, err = outFile.Write(buf.Bytes())
		check(err, "failed to write to %v: %v", outFilePath, err)

		err = outFile.Close()
		check(err, "failed to close %v: %v", outFilePath, err)
	}
}

func (r *runner) generateTestLog(g *guide) {
	for lang, ls := range g.Langs {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Image: %v\n", g.Image)
		if len(g.Presteps) > 0 {
			byts, err := json.MarshalIndent(g.Presteps, "", "  ")
			check(err, "failed to marshal prestep: %v", err)
			fmt.Fprintf(&buf, "Presteps: %s\n", byts)
		}
		for _, step := range ls.steps {
			step.renderTestLog(&buf)
		}
		logFilePath := filepath.Join(g.dir, fmt.Sprintf("%v_testlog.txt", lang))
		err := ioutil.WriteFile(logFilePath, buf.Bytes(), 0666)
		check(err, "failed to write testlog output to %v: %v", logFilePath, err)
	}
}

func (g *guide) sanitiseVars(s string) string {
	for _, repl := range g.vars {
		parts := strings.SplitN(repl, "=", 2)
		v, val := parts[0], parts[1]
		s = strings.ReplaceAll(s, val, "$"+v)
	}
	return s
}
