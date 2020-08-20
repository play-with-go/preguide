package out

import "github.com/play-with-go/preguide"

#GuideOutput: {
	#GuideStructure
	Langs: [preguide.#Language]: #LangSteps
	Defs: [string]:              _
}

_#stepCommon: {
	StepType: #StepType
	Name:     string
	Order:    int
	Terminal: string
	...
}

// TODO: keep this in sync with the Go definitions
#StepType: int

#StepTypeCommand: #StepType & 1
#StepTypeUpload:  #StepType & 2

#Prestep: {
	Package: string
	Path:    string
	Args:    _
	Version: string
}

#LangSteps: {
	Hash: string
	Steps: [string]: #Step
}

#Step: (#CommandStep | #UploadStep) & _#stepCommon

#CommandStep: {
	_#stepCommon
	Stmts: [...#Stmt]
}

#Terminal: {
	Name:  string
	Image: string
}

#Stmt: {
	Negated:  bool
	CmdStr:   string
	ExitCode: int
	Output:   string
}

#UploadStep: {
	_#stepCommon
	Source: string
	Target: string
}

// GuideStructures maps a guide name to its #GuideStructure
#GuideStructures: [string]: #GuideStructure

// A #GuideStructure defines the prestep and terminal
// structure of a guide.
#GuideStructure: {
	Terminals: [...#Terminal]
	Presteps: [...#Prestep]
}
