package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"gopkg.in/yaml.v2"
)

type guide struct {
	dir    string
	name   string
	target string

	mdFiles []mdFile
	langs   []string
	Image   string
	Langs   map[string]*langSteps

	outputGuide *guide
	output      cue.Value
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
		// TODO: support rewriting of directives
		// if len(md.directives) != 0 {
		// 	raise("we don't yet support directive parsing")
		// }

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
			steps := g.Langs[md.lang].Steps
			pos := 0
			for _, d := range md.directives {
				buf.Write(md.content[pos:d.pos])
				if *r.fCompat {
					steps[d.key].renderCompat(&buf)
				} else {
					steps[d.key].render(&buf)
				}
				pos = d.end
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
		for _, step := range ls.steps {
			step.renderTestLog(&buf)
		}
		logFilePath := filepath.Join(g.dir, fmt.Sprintf("%v_testlog.txt", lang))
		err := ioutil.WriteFile(logFilePath, buf.Bytes(), 0666)
		check(err, "failed to write testlog output to %v: %v", logFilePath, err)
	}
}
