// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package types

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/play-with-go/preguide"
	"github.com/play-with-go/preguide/internal/textutil"
)

type StepType int64

const (
	// TODO: keep this in sync with the CUE definitions
	StepTypeCommand StepType = iota + 1
	StepTypeCommandFile
	StepTypeUpload
	StepTypeUploadFile
)

type Mode string

const (
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
	case ModeJekyll, ModeGitHub:
	default:
		return fmt.Errorf("unknown mode %q", v)
	}
	*m = Mode(v)
	return nil
}

type Guide struct {
	Languages []string
	Delims    [2]string
	Presteps  []*preguide.Prestep
	Steps     Steps
	Terminals map[string]*preguide.Terminal
	Scenarios map[string]*preguide.Scenario
	Defs      map[string]interface{}
	Networks  []string
	Env       []string
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
			return fmt.Errorf("failed to unmarshal Step: %v", err)
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
	case StepTypeCommandFile:
		s = new(CommandFile)
	case StepTypeUpload:
		s = new(Upload)
	case StepTypeUploadFile:
		s = new(UploadFile)
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
	StepTypeVal   StepType `json:"StepType"`
	Terminal      string
	Name          string
	RandomReplace *string
	DoNotTrim     bool
	Source        string
}

var _ Step = (*Command)(nil)

func (c *Command) StepType() StepType {
	return c.StepTypeVal
}

type CommandFile struct {
	StepTypeVal   StepType `json:"StepType"`
	Terminal      string
	Name          string
	RandomReplace *string
	DoNotTrim     bool
	Path          string
}

var _ Step = (*CommandFile)(nil)

func (c *CommandFile) StepType() StepType {
	return c.StepTypeVal
}

type Upload struct {
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Target      string
	Language    string
	Renderer    Renderer
	Source      string
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

type UploadFile struct {
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Target      string
	Language    string
	Renderer    Renderer
	Path        string
}

var _ Step = (*UploadFile)(nil)

func (u *UploadFile) StepType() StepType {
	return u.StepTypeVal
}

func (u *UploadFile) UnmarshalJSON(b []byte) error {
	type noUnmarshall UploadFile
	var uv struct {
		*noUnmarshall
		Renderer json.RawMessage
	}
	uv.noUnmarshall = (*noUnmarshall)(u)
	if err := json.Unmarshal(b, &uv); err != nil {
		return fmt.Errorf("failed to unmarshal wrapped UploadFile: %v", err)
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
	return strings.Join(res, "\n"), nil
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
	same := func(w io.Writer, s string) {
		fmt.Fprintf(w, "%s\n", s)
	}
	before := func(w io.Writer, s string) {}
	after := func(w io.Writer, s string) {
		fmt.Fprintf(w, "<b style=\"color:darkblue\">%s</b>\n", s)
	}
	res := textutil.Diff(r.Pre, v, same, before, after)
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
