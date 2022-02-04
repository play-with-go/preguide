// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"strings"
	"text/template"

	"github.com/play-with-go/preguide/internal/types"
	"github.com/play-with-go/preguide/sanitisers"
	"mvdan.cc/sh/v3/syntax"
)

type steps map[string]step

func (l *steps) UnmarshalJSON(b []byte) error {
	var v map[string]json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("failed to unmarshal steps into wrapper: %v", err)
	}
	if len(v) > 0 && *l == nil {
		*l = make(map[string]step)
	}
	for stepName, stepBytes := range v {
		s, err := unmarshalStep(stepBytes)
		if err != nil {
			return fmt.Errorf("failed to unmarshal step for step %v: %v", stepName, err)
		}
		(*l)[stepName] = s
	}
	return nil
}

func unmarshalStep(r json.RawMessage) (step, error) {
	var discrim struct {
		StepType StepType
	}
	if err := json.Unmarshal(r, &discrim); err != nil {
		return nil, fmt.Errorf("failed to unmarshal disciminator type: %v", err)
	}
	var s step
	switch discrim.StepType {
	case StepTypeCommand:
		s = new(commandStep)
	case StepTypeUpload:
		s = new(uploadStep)
	default:
		panic(fmt.Errorf("unknown StepType: %v", discrim.StepType))
	}
	if err := json.Unmarshal(r, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %T: %v", s, err)
	}
	return s, nil
}

type step interface {
	name() string
	order() int
	terminal() string
	setorder(int)
	render(io.Writer, renderOptions)
	renderLog(types.Mode, io.Writer)
	setOutputFrom(step)
	mustBeReferenced() bool
}

type renderOptions struct {
	mode            types.Mode
	FilenameComment bool
}

type commandStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	StepType        StepType
	RandomReplace   *string
	DoNotTrim       bool
	InformationOnly bool
	Name            string
	Order           int
	Terminal        string

	Stmts []*commandStmt
}

func newCommandStep(cs commandStep) *commandStep {
	cs.StepType = StepTypeCommand
	return &cs
}

func (c *commandStep) name() string {
	return c.Name
}

func (c *commandStep) order() int {
	return c.Order
}

func (c *commandStep) terminal() string {
	return c.Terminal
}

func (c *commandStep) setorder(i int) {
	c.Order = i
}

type commandStmt struct {
	Negated          bool
	CmdStr           string
	ExitCode         int
	Output           string
	ComparisonOutput string
	outputFence      string

	sanitiser sanitisers.Sanitiser
}

// commandStepFromCommand takes a string value that is a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case s cannot be parsed
func (pdc *processDirContext) commandStepFromCommand(s *types.Command) (*commandStep, error) {
	r := strings.NewReader(s.Source)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command string %q: %v", s.Source, err)
	}
	res := newCommandStep(commandStep{
		Name:            s.Name,
		RandomReplace:   s.RandomReplace,
		InformationOnly: s.InformationOnly,
		DoNotTrim:       s.DoNotTrim,
		Terminal:        s.Terminal,
	})
	return pdc.commadStepFromSyntaxFile(res, f)
}

// commandStepFromCommandFile takes a path to a file that contains a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case path cannot be read or parsed
func (pdc *processDirContext) commandStepFromCommandFile(s *types.CommandFile) (*commandStep, error) {
	byts, err := ioutil.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", s.Path, err)
	}
	r := bytes.NewReader(byts)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse commands from %v: %v", s.Path, err)
	}
	res := newCommandStep(commandStep{
		Name:            s.Name,
		RandomReplace:   s.RandomReplace,
		InformationOnly: s.InformationOnly,
		DoNotTrim:       s.DoNotTrim,
		Terminal:        s.Terminal,
	})
	return pdc.commadStepFromSyntaxFile(res, f)
}

// commadStepFromSyntaxFile takes a *mvdan.cc/sh/syntax.File and returns a
// commandStep with the individual statements, or an error in case any of the
// statements cannot be printed as string values
func (pdc *processDirContext) commadStepFromSyntaxFile(res *commandStep, f *syntax.File) (*commandStep, error) {
	res.StepType = StepTypeCommand
	for i, stmt := range f.Stmts {
		// Capture whether this statement is negated or not
		negated := stmt.Negated
		// Set to not negated because we need to capture the exit code.
		// Handling of the exit code and negated happens in the generated
		// bash script
		stmt.Negated = false
		var sb strings.Builder
		if err := pdc.stmtPrinter.Print(&sb, stmt); err != nil {
			return res, fmt.Errorf("failed to print statement %v: %v", i, err)
		}
		var sans []sanitisers.Sanitiser
		for _, d := range stmtSanitisers {
			if san := d(pdc.sanitiserHelper, stmt); san != nil {
				sans = append(sans, san)
			}
		}
		var san sanitisers.Sanitiser
		switch len(sans) {
		case 0:
		case 1:
			san = sans[0]
		default:
			return nil, fmt.Errorf("statement %v resulted in multiple sanitisers", stmt.Cmd)
		}
		res.Stmts = append(res.Stmts, &commandStmt{
			CmdStr:    sb.String(),
			Negated:   negated,
			sanitiser: san,
		})
	}
	return res, nil
}

func (c *commandStep) render(w io.Writer, opts renderOptions) {
	var cmds, encCmds bytes.Buffer
	enc := base64.NewEncoder(base64.StdEncoding, &encCmds)
	if len(c.Stmts) > 0 {
		var stmt *commandStmt
		for _, stmt = range c.Stmts {
			fmt.Fprintf(enc, "%s\n", stmt.CmdStr)
			// replaceBraces is safe to do here because in all modes we are
			// outputting <pre><code> blocks
			fmt.Fprintf(&cmds, "$ %s\n", stmt.CmdStr)
			fmt.Fprintf(&cmds, "%s", stmt.Output)
		}
		// Output a trailing newline if the last block of output did not include one
		// otherwise the closing code block fence will not render properly
		if stmt.Output != "" && stmt.Output[len(stmt.Output)-1] != '\n' {
			fmt.Fprintf(&cmds, "\n")
		}
	}
	enc.Close()
	switch opts.mode {
	case types.ModeJekyll:
		fmt.Fprintf(w, "<pre data-command-src=\"%s\"><code class=\"language-%v\">", encCmds.Bytes(), "."+c.Terminal)
	case types.ModeGitHub:
		// Note we are not using language syntax highlighting here because we
		// prefer to be able to use <b> and <i> for diff and filenames respectively
		fmt.Fprintf(w, "<pre><code>")
	}
	cmdsStr := cmds.String()
	cmdsStr = template.HTMLEscapeString(cmdsStr)
	cmdsStr = replaceBraces(cmdsStr)
	fmt.Fprintf(w, "%s", cmdsStr)
	fmt.Fprintf(w, "</code></pre>")
}

func (c *commandStep) renderLog(mode types.Mode, w io.Writer) {
	if len(c.Stmts) > 0 {
		var stmt *commandStmt
		for _, stmt = range c.Stmts {
			fmt.Fprintf(w, "$ %s\n", stmt.CmdStr)
			fmt.Fprintf(w, "%s", stmt.Output)
		}
		// Output a trailing newline if the last block of output did not include one
		// otherwise the closing code block fence will not render properly
		if stmt.Output != "" && stmt.Output[len(stmt.Output)-1] != '\n' {
			fmt.Fprintf(w, "\n")
		}
	}
}

func (c *commandStep) setOutputFrom(s step) {
	oc, ok := s.(*commandStep)
	if !ok {
		panic(fmt.Errorf("expected a *commandStep; got %T", s))
	}
	for i, s := range oc.Stmts {
		c.Stmts[i].ExitCode = s.ExitCode
		c.Stmts[i].Output = s.Output
		c.Stmts[i].ComparisonOutput = s.ComparisonOutput
	}
}

func (c *commandStep) mustBeReferenced() bool {
	return !c.InformationOnly
}

func trimTrailingNewline(s string) string {
	trimmed := s
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

type uploadStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	StepType StepType
	Name     string
	Order    int
	Terminal string
	Language string
	Renderer types.Renderer

	Source string
	Target string
}

func newUploadStep(u uploadStep) *uploadStep {
	u.StepType = StepTypeUpload
	return &u
}

func (u *uploadStep) name() string {
	return u.Name
}

func (u *uploadStep) order() int {
	return u.Order
}

func (u *uploadStep) terminal() string {
	return u.Terminal
}

func (u *uploadStep) setorder(i int) {
	u.Order = i
}

func (u *uploadStep) UnmarshalJSON(b []byte) error {
	type noUnmarshall uploadStep
	var uv struct {
		*noUnmarshall
		Renderer json.RawMessage
	}
	uv.noUnmarshall = (*noUnmarshall)(u)
	if err := json.Unmarshal(b, &uv); err != nil {
		return fmt.Errorf("failed to unmarshal wrapped uploadStep: %v", err)
	}
	r, err := types.UnmarshalRenderer(uv.Renderer)
	if err != nil {
		return err
	}
	u.Renderer = r
	return nil
}

func (pdc *processDirContext) uploadStepFromUpload(u *types.Upload) (*uploadStep, error) {
	res := newUploadStep(uploadStep{
		Name:     u.Name,
		Terminal: u.Terminal,
		Language: u.Language,
		Renderer: u.Renderer,
		Target:   u.Target,
		Source:   u.Source,
	})
	return res, nil
}

func (pdc *processDirContext) uploadStepFromUploadFile(u *types.UploadFile) (*uploadStep, error) {
	byts, err := ioutil.ReadFile(u.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", u.Path, err)
	}
	res := newUploadStep(uploadStep{
		Name:     u.Name,
		Terminal: u.Terminal,
		Language: u.Language,
		Renderer: u.Renderer,
		Target:   u.Target,
		Source:   string(byts),
	})
	return res, nil
}

func (u *uploadStep) render(w io.Writer, opts renderOptions) {
	origSource, err := u.Renderer.Render(opts.mode, u.Source)
	check(err, "failed to render upload step: %v", err)

	// Special case GitHub for now
	if opts.mode == types.ModeGitHub {
		fmt.Fprintf(w, "```%s", u.Language)
		if opts.FilenameComment {
			fmt.Fprintf(w, "\n%s\n", comment(opts.mode, u.Target, u.Language))
		}
		fmt.Fprintf(w, "\n%s", origSource)
		if len(origSource) > 0 && !strings.HasSuffix(origSource, "\n") {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "```")
		return
	}

	renderedSource := replaceBraces(origSource)
	source := base64Encode(u.Source)
	// Workaround github.com/play-with-go/play-with-go/issues/44 by encoding the
	// target as base64 in case it contains any {{.BLAH}} templates.  The
	// frontend half of this workaround will do the decoding before any
	// attempted replacement of the substitution happens.
	targetDir := base64Encode(path.Dir(u.Target))
	targetFile := base64Encode(path.Base(u.Target))
	switch opts.mode {
	case types.ModeJekyll:
		fmt.Fprintf(w, "<pre data-upload-path=\"%v\" data-upload-src=\"%v:%v\" data-upload-term=\"%v\"><code class=\"language-%v\">", targetDir, targetFile, source, "."+u.Terminal, u.Language)
	}
	if opts.FilenameComment {
		fmt.Fprintf(w, "<i class=\"filename\">%s</i>\n\n", comment(opts.mode, u.Target, u.Language))
	}
	fmt.Fprintf(w, "%s", renderedSource)
	fmt.Fprintf(w, "</code></pre>")
}

func (u *uploadStep) mustBeReferenced() bool {
	return true
}

func base64Encode(s string) string {
	var buf bytes.Buffer
	targetDirEnv := base64.NewEncoder(base64.StdEncoding, &buf)
	targetDirEnv.Write([]byte(s))
	targetDirEnv.Close()
	return buf.String()
}

func (u *uploadStep) renderLog(mode types.Mode, w io.Writer) {
	fmt.Fprintf(w, "$ cat <<EOD > %v\n%s\nEOD\n", u.Target, u.Source)
}

func (u *uploadStep) setOutputFrom(s step) {
}

func replaceBraces(s string) string {
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")
	return s
}

func comment(mode types.Mode, s string, lang string) (res string) {
	switch lang {
	case "go", "go.mod", "cue":
		res = linewiseComment(s, "// ")
	case "sh", "bash", "txt", "toml", "yaml":
		res = linewiseComment(s, "# ")
	case "markdown", "md", "mkd", "mkdn", "mdown": // sync with markdownFile regex
		res = fmt.Sprintf("<!-- %v -->", s)
	default:
		raise("don't know how to comment language %v", lang)
	}
	if mode == types.ModeJekyll {
		res = template.HTMLEscapeString(res)
	}
	return res
}

func linewiseComment(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	end := len(lines)
	if lines[end-1] == "" {
		end--
	}
	for i, l := range lines[:end] {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
