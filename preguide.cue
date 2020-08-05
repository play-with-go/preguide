package preguide

// TODO: keep this in sync with the Go definitions
#StepType: int

#StepTypeCommand:     #StepType & 1
#StepTypeCommandFile: #StepType & 2
#StepTypeUpload:      #StepType & 3
#StepTypeUploadFile:  #StepType & 4

#Guide: {

	#Step: (#Command | #CommandFile | #Upload | #UploadFile ) & _#stepCommon

	_#stepCommon: {
		StepType: #StepType
		...
	}

	#Command: {
		_#stepCommon
		StepType: #StepTypeCommand
		Source:   string
	}

	#CommandFile: {
		_#stepCommon
		StepType: #StepTypeCommandFile
		Path:     string
	}

	#Upload: {
		_#stepCommon
		StepType: #StepTypeUpload
		Target:   string
		Source:   string
	}

	#UploadFile: {
		_#stepCommon
		StepType: #StepTypeUploadFile
		Target:   string
		Path:     string
	}

	Presteps: [...#Prestep]

	// Delims are the delimiters used in the guide prose and steps
	// for environment variable substitution. A template substitution
	// of the environment variable ABC therefore looks like "{{ .ABC }}"
	Delims: *["{{", "}}"] | [string, string]

	// Images are optional because a guide does not need to have
	// any steps. However, we can't make this conditional on len(Steps) because
	// of cuelang.org/issue/279. Hence we validate in code.
	Image?: string

	Steps: [string]: en: #Step

	Defs: [string]: _
}

#Prestep: {
	Package: string
	Args: [...string]
}

// The following definitions necessarily reference the nested definitions
// in #Guide, because those definitions rely on references to Terminals
// which only makes sense in the context of a #Guide instance

#Step:        #Guide.#Step
#Command:     #Guide.#Command
#CommandFile: #Guide.#CommandFile
#Upload:      #Guide.#Upload
#UploadFile:  #Guide.#UploadFile
