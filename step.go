package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/play-with-go/preguide/internal/types"
	"mvdan.cc/sh/v3/syntax"
)

type langSteps struct {
	Steps      map[string]step
	bashScript string
	Hash       string
	steps      []step
}

func (l *langSteps) UnmarshalJSON(b []byte) error {
	type noUnmarshal langSteps
	var v struct {
		Steps map[string]json.RawMessage
		*noUnmarshal
	}
	v.noUnmarshal = (*noUnmarshal)(l)
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("failed to unmarshal langSteps into wrapper: %v", err)
	}
	if len(v.Steps) > 0 && l.Steps == nil {
		l.Steps = make(map[string]step)
	}
	for stepName, stepBytes := range v.Steps {
		s, err := unmarshalStep(stepBytes)
		if err != nil {
			return fmt.Errorf("failed to unmarshal step for step %v: %v", stepName, err)
		}
		l.Steps[stepName] = s
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

func newLangSteps() *langSteps {
	return &langSteps{
		Steps: make(map[string]step),
	}
}

type step interface {
	name() string
	order() int
	terminal() string
	setorder(int)
	render(io.Writer)
	renderCompat(io.Writer)
	renderLog(io.Writer)
}

type commandStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	StepType StepType
	Name     string
	Order    int
	Terminal string

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
	Negated     bool
	CmdStr      string
	ExitCode    int
	Output      string
	outputFence string

	sanitisers []sanitiser
}

// commandStepFromCommand takes a string value that is a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case s cannot be parsed
func commandStepFromCommand(s *types.Command) (*commandStep, error) {
	r := strings.NewReader(s.Source)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse command string %q: %v", s, err)
	}
	res := newCommandStep(commandStep{
		Name:     s.Name,
		Terminal: s.Terminal,
	})
	return commadStepFromSyntaxFile(res, f)
}

// commandStepFromCommandFile takes a path to a file that contains a sequence of shell
// statements and returns a commandStep with the individual parsed statements,
// or an error in case path cannot be read or parsed
func commandStepFromCommandFile(s *types.CommandFile) (*commandStep, error) {
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
		Name:     s.Name,
		Terminal: s.Terminal,
	})
	return commadStepFromSyntaxFile(res, f)
}

// commadStepFromSyntaxFile takes a *mvdan.cc/sh/syntax.File and returns a
// commandStep with the individual statements, or an error in case any of the
// statements cannot be printed as string values
func commadStepFromSyntaxFile(res *commandStep, f *syntax.File) (*commandStep, error) {
	res.StepType = StepTypeCommand
	printer := syntax.NewPrinter()
	sm := sanitiserMatcher{
		printer: printer,
	}
	for i, stmt := range f.Stmts {
		// Capture whether this statement is negated or not
		negated := stmt.Negated
		// Set to not negated because we need to capture the exit code.
		// Handling of the exit code and negated happens in the generated
		// bash script
		stmt.Negated = false
		var sb strings.Builder
		if err := printer.Print(&sb, stmt); err != nil {
			return res, fmt.Errorf("failed to print statement %v: %v", i, err)
		}
		res.Stmts = append(res.Stmts, &commandStmt{
			CmdStr:     sb.String(),
			Negated:    negated,
			sanitisers: sm.deriveSanitiser(stmt),
		})
	}
	return res, nil
}

func (c *commandStep) render(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	if len(c.Stmts) > 0 {
		var stmt *commandStmt
		for _, stmt = range c.Stmts {
			fmt.Fprintf(w, "$ %s\n", stmt.CmdStr)
			fmt.Fprintf(w, "%s", stmt.Output)
		}
		// Output a trailing newline if the last block of output did not include one
		// otherwise the closing code block fence will not render properly
		if stmt.Output[len(stmt.Output)-1] != '\n' {
			fmt.Fprintf(w, "\n")
		}
	}
	fmt.Fprintf(w, "```")
}

func (c *commandStep) renderCompat(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	if len(c.Stmts) > 0 {
		var stmt *commandStmt
		for _, stmt = range c.Stmts {
			fmt.Fprintf(w, "%s\n", stmt.CmdStr)
		}
		// Output a trailing newline if the last block of output did not include one
		// otherwise the closing code block fence will not render properly
		if stmt.Output != "" && stmt.Output[len(stmt.Output)-1] != '\n' {
			fmt.Fprintf(w, "\n")
		}
	}
	fmt.Fprintf(w, "```")
}

func (c *commandStep) renderLog(w io.Writer) {
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

type uploadStep struct {
	// Extract once we have a solution to cuelang.org/issue/376
	StepType StepType
	Name     string
	Order    int
	Terminal string

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

func uploadStepFromUpload(u *types.Upload) (*uploadStep, error) {
	res := newUploadStep(uploadStep{
		Name:   u.Name,
		Source: u.Source,
		Target: u.Target,
	})
	return res, nil
}

func uploadStepFromUploadFile(u *types.UploadFile) (*uploadStep, error) {
	byts, err := ioutil.ReadFile(u.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", u.Path, err)
	}
	res := newUploadStep(uploadStep{
		Name:   u.Name,
		Source: string(byts),
		Target: u.Target,
	})
	return res, nil
}

func (u *uploadStep) render(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	fmt.Fprintf(w, "%s\n", u.Source)
	fmt.Fprintf(w, "```")
}

func (u *uploadStep) renderCompat(w io.Writer) {
	fmt.Fprintf(w, "```.term1\n")
	source := strings.ReplaceAll(u.Source, "\t", "        ")
	fmt.Fprintf(w, "cat <<EOD > %v\n%s\nEOD\n", u.Target, source)
	fmt.Fprintf(w, "```")
}

func (u *uploadStep) renderLog(w io.Writer) {
	fmt.Fprintf(w, "$ cat <<EOD > %v\n%s\nEOD\n", u.Target, u.Source)
}
