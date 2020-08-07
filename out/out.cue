package out

import "github.com/play-with-go/preguide"

#GuideOutput: {
	Presteps: [...#Prestep]
	Image: string

	Langs: [preguide.#Language]: #LangSteps

	Defs: [string]: _
}

_#stepCommon: {
	StepType: #StepType
	Name:     string
	Order:    int
	...
}

// TODO: keep this in sync with the Go definitions
#StepType: int

#StepTypeCommand: #StepType & 1
#StepTypeUpload:  #StepType & 2

#Prestep: {
	Package: string
	Args: [...string]
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
