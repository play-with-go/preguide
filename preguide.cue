package preguide

#Guide: {
	Presteps: [...#Prestep]

	// Delims are the delimiters used in the guide prose and steps
	// for environment variable substitution. A template substitution
	// of the environment variable ABC therefore looks like "{{ .ABC }}"
	Delims: *["{{", "}}"] | [string, string]

	// Images are optional because a guide does not need to have
	// any steps. However, we can't make this conditional on len(Steps) because
	// of cuelang.org/issue/279. Hence we validate in code.
	Image?: string

	Steps: [string]: en: (#Command | #CommandFile | #Upload | #UploadFile) & {
		StepType: #StepType
	}
	Defs: [string]: _
}

#StepType: int

#StepTypeCommand:     #StepType & 1
#StepTypeCommandFile: #StepType & 2
#StepTypeUpload:      #StepType & 3
#StepTypeUploadFile:  #StepType & 4

#Prestep: {
	Package: string
	Args: [...string]
}

#Command: {
	StepType: #StepTypeCommand
	Source:   string
}

#CommandFile: {
	StepType: #StepTypeCommandFile
	Path:     string
}

#Upload: {
	StepType: #StepTypeUpload
	Target:   string
	Source:   string
}

#UploadFile: {
	StepType: #StepTypeUploadFile
	Target:   string
	Path:     string
}
