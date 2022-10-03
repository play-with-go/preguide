// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package types

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/play-with-go/preguide"
	"github.com/play-with-go/preguide/internal/textutil"
)

type StepType int64

const (
	// TODO: keep this in sync with the CUE definitions
	StepTypeCommand StepType = 1
	StepTypeUpload  StepType = 3
)

type Mode string

const (
	ModeRaw    Mode = "raw"
	ModeJekyll Mode = "jekyll"
	ModeGitHub Mode = "github"
)

func (m *Mode) String() string {
	if m == nil {
		return "nil"
	}
	return string(*m)
}

func (m *Mode) Set(v string) error {
	switch Mode(v) {
	case ModeJekyll, ModeGitHub, ModeRaw:
	default:
		return fmt.Errorf("unknown mode %q", v)
	}
	*m = Mode(v)
	return nil
}

type Guide struct {
	Languages       []string
	Delims          [2]string
	Presteps        []*preguide.Prestep
	FilenameComment *bool
	Steps           Steps
	Terminals       map[string]*preguide.Terminal
	Scenarios       map[string]*preguide.Scenario
	Defs            map[string]interface{}
	Networks        []string
	Env             []string
}

type LangCode string

func ValidLangCode(s string) bool {
	for _, code := range Langs {
		if code == LangCode(s) {
			return true
		}
	}
	return false
}

type Steps map[string]Step

func (l *Steps) UnmarshalJSON(b []byte) error {
	var v map[string]json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v) > 0 && *l == nil {
		*l = make(map[string]Step)
	}
	for stepName, m := range v {
		var s Step
		s, err := unmarshalStep(m)
		if err != nil {
			return fmt.Errorf("failed to unmarshal Step %q: %v", stepName, err)
		}
		(*l)[stepName] = s
	}
	return nil
}

func unmarshalStep(r json.RawMessage) (Step, error) {
	var discrim struct {
		StepType StepType
	}
	if err := json.Unmarshal(r, &discrim); err != nil {
		return nil, fmt.Errorf("failed to unmarshal disciminator type: %v", err)
	}
	var s Step
	switch discrim.StepType {
	case StepTypeCommand:
		s = new(Command)
	case StepTypeUpload:
		s = new(Upload)
	default:
		panic(fmt.Errorf("unknown StepType: %v", discrim.StepType))
	}
	if err := json.Unmarshal(r, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %T: %v", s, err)
	}
	return s, nil
}

type Step interface {
	StepType() StepType
}

type Command struct {
	StepTypeVal     StepType `json:"StepType"`
	Terminal        string
	Name            string
	InformationOnly *bool
	Stmts           Stmts
	Path            *string
}

func (u *Command) UnmarshalJSON(b []byte) error {
	type noUnmarshall Command
	var uv struct {
		*noUnmarshall
		Stmts json.RawMessage
	}
	uv.noUnmarshall = (*noUnmarshall)(u)
	if err := json.Unmarshal(b, &uv); err != nil {
		return fmt.Errorf("failed to unmarshal wrapped Command: %v", err)
	}
	r, err := UnmarshalStmts(uv.Stmts)
	if err != nil {
		return err
	}
	u.Stmts = r
	return nil
}

func UnmarshalStmts(v json.RawMessage) (Stmts, error) {
	if v == nil {
		return nil, nil
	}
	// Try to unmarshal string first
	var css StmtsString
	if err := json.Unmarshal(v, &css); err == nil {
		return css, nil
	}
	// Now try a list. Is this fails that's an error
	var cslraw []json.RawMessage
	if err := json.Unmarshal(v, &cslraw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal; not string or list")
	}
	// We have a list at this point. We need to ensure that
	// each element is the correct type
	var csl StmtsList
	for i, v := range cslraw {
		// Try to unmarshal a string first
		var csles StmtsListElemString
		if err := json.Unmarshal(v, &csles); err == nil {
			csl = append(csl, csles)
			continue
		}
		// Now try a Cmd
		var cslecmd Stmt
		if err := json.Unmarshal(v, &cslecmd); err == nil {
			csl = append(csl, cslecmd)
			continue
		}
		return nil, fmt.Errorf("failed to unmarshal element %d (%s); not string or Cmd", i, v)
	}
	return csl, nil
}

type Stmts interface {
	isStmts()
}

type StmtsString string

var _ Stmts = StmtsString("")

func (c StmtsString) isStmts() {}

type StmtsList []StmtsListElem

var _ Stmts = StmtsList(nil)

func (c StmtsList) isStmts() {}

type StmtsListElem interface {
	isStmtsListElem()
}

type StmtsListElemString string

var _ StmtsListElem = StmtsListElemString("")

func (c StmtsListElemString) isStmtsListElem() {}

type Stmt struct {
	Cmd           *string
	RandomReplace *string
	DoNotTrim     *bool
}

var _ StmtsListElem = Stmt{}

func (c Stmt) isStmtsListElem() {}

var _ Step = (*Command)(nil)

func (c *Command) StepType() StepType {
	return c.StepTypeVal
}

type Upload struct {
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Target      string
	Language    string
	Renderer    Renderer
	Source      *string
	Path        *string
}

var _ Step = (*Upload)(nil)

func (u *Upload) StepType() StepType {
	return u.StepTypeVal
}

func (u *Upload) UnmarshalJSON(b []byte) error {
	type noUnmarshall Upload
	var uv struct {
		*noUnmarshall
		Renderer json.RawMessage
	}
	uv.noUnmarshall = (*noUnmarshall)(u)
	if err := json.Unmarshal(b, &uv); err != nil {
		return fmt.Errorf("failed to unmarshal wrapped Upload: %v", err)
	}
	r, err := UnmarshalRenderer(uv.Renderer)
	if err != nil {
		return err
	}
	u.Renderer = r
	return nil
}

type RendererType int64

const (
	// TODO: keep this in sync with the CUE definitions
	RendererTypeFull RendererType = iota + 1
	RendererTypeLineRanges
	RendererTypeDiff
)

type Renderer interface {
	rendererType() RendererType
	Render(Mode, string) (string, error)
}

type RendererFull struct {
	RendererType RendererType
}

var _ Renderer = (*RendererFull)(nil)

func newRendererFull(r RendererFull) *RendererFull {
	r.RendererType = RendererTypeFull
	return &r
}

func (r *RendererFull) rendererType() RendererType {
	return RendererTypeFull
}

func (r *RendererFull) Render(m Mode, v string) (string, error) {
	switch m {
	case ModeJekyll:
		v = template.HTMLEscapeString(v)
	}
	return v, nil
}

type RendererLineRanges struct {
	RendererType RendererType
	Ellipsis     string
	Lines        [][2]int64
}

var _ Renderer = (*RendererLineRanges)(nil)

func newRendererLineRanges(r RendererLineRanges) *RendererLineRanges {
	r.RendererType = RendererTypeLineRanges
	return &r
}

func (r *RendererLineRanges) rendererType() RendererType {
	return RendererTypeLineRanges
}

func (r *RendererLineRanges) Render(m Mode, v string) (string, error) {
	lines := strings.Split(v, "\n")
	l := int64(len(lines))
	var res []string
	for _, rng := range r.Lines {
		if rng[0] > l || rng[1] > l {
			return "", fmt.Errorf("range %v is outside the number of actual lines: %v (%q)", rng, l, v)
		}
		if rng[0] > 1 {
			res = append(res, r.Ellipsis)
		}
		res = append(res, lines[rng[0]-1:rng[1]]...)
		if rng[1]-1 < l-1 {
			res = append(res, r.Ellipsis)
		}
	}
	result := strings.Join(res, "\n")
	switch m {
	case ModeJekyll:
		result = template.HTMLEscapeString(result)
	}
	return result, nil
}

type RendererDiff struct {
	RendererType RendererType
	Pre          string
}

var _ Renderer = (*RendererDiff)(nil)

func newRendererDiff(r RendererDiff) *RendererDiff {
	r.RendererType = RendererTypeDiff
	return &r
}

func (r *RendererDiff) rendererType() RendererType {
	return RendererTypeDiff
}

func (r *RendererDiff) Render(m Mode, v string) (string, error) {
	// Hack: for now if we are asked to output in GitHub
	// mode, fail. Because we haven't yet worked out how,
	// if at all, to support showing diffs well in syntax
	// highlighted code blocks.
	if m == ModeGitHub {
		return "", fmt.Errorf("cannot render diff in %v", m)
	}
	same := func(w io.Writer, s string) {
		fmt.Fprintf(w, "%s\n", s)
	}
	before := func(w io.Writer, s string) {}
	after := func(w io.Writer, s string) {
		fmt.Fprintf(w, "<b>%s</b>\n", s)
	}
	pre, v := r.Pre, v
	switch m {
	case ModeJekyll:
		pre, v = template.HTMLEscapeString(pre), template.HTMLEscapeString(v)
	}
	res := textutil.Diff(pre, v, false, same, before, after)
	return res, nil
}

func UnmarshalRenderer(v json.RawMessage) (Renderer, error) {
	if string(v) == "{}" {
		panic("here")
	}
	var discrim struct {
		RendererType RendererType
	}
	if err := json.Unmarshal(v, &discrim); err != nil {
		return nil, fmt.Errorf("failed to unmarshal disciminator type: %v", err)
	}
	var r Renderer
	switch discrim.RendererType {
	case RendererTypeFull:
		r = newRendererFull(RendererFull{})
	case RendererTypeLineRanges:
		r = newRendererLineRanges(RendererLineRanges{})
	case RendererTypeDiff:
		r = newRendererDiff(RendererDiff{})
	default:
		panic(fmt.Errorf("unknown RendererType: %v", discrim.RendererType))
	}
	if err := json.Unmarshal(v, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %T: %v", r, err)
	}
	return r, nil
}
