package types

import (
	"encoding/json"
	"fmt"
)

type StepType int64

const (
	// TODO: keep this in sync with the CUE definitions
	StepTypeCommand StepType = iota + 1
	StepTypeCommandFile
	StepTypeUpload
	StepTypeUploadFile
)

type Guide struct {
	Delims    [2]string
	Presteps  []*Prestep
	Steps     map[string]LangSteps
	Terminals map[string]*Terminal
	Defs      map[string]interface{}
}

type Terminal struct {
	Name  string
	Image string
}

type Prestep struct {
	Package string
	Args    []string
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

type LangSteps map[LangCode]Step

func (l *LangSteps) UnmarshalJSON(b []byte) error {
	var v map[LangCode]json.RawMessage
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v) > 0 && *l == nil {
		*l = make(map[LangCode]Step)
	}
	for code, m := range v {
		var s Step
		s, err := unmarshalStep(m)
		if err != nil {
			return fmt.Errorf("failed to unmarshal Step for En: %v", err)
		}
		(*l)[code] = s
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
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Source      string
}

var _ Step = (*Command)(nil)

func (c *Command) StepType() StepType {
	return c.StepTypeVal
}

type CommandFile struct {
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Path        string
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
	Source      string
}

var _ Step = (*Upload)(nil)

func (c *Upload) StepType() StepType {
	return c.StepTypeVal
}

type UploadFile struct {
	StepTypeVal StepType `json:"StepType"`
	Terminal    string
	Name        string
	Target      string
	Path        string
}

var _ Step = (*UploadFile)(nil)

func (c *UploadFile) StepType() StepType {
	return c.StepTypeVal
}
