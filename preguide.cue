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

	Steps: [string]: en: #Command | #CommandFile | #Upload | #UploadFile
	Defs: [string]: _
}

#Prestep: {
	Package: string
	Args: [...string]
}

#Command: {
	#IsCommand: true
	Source:     string
}

#CommandFile: {
	#IsCommandFile: true
	Path:           string
}

#Upload: {
	#IsUpload: true
	Target:    string
	Source:    string
}

#UploadFile: {
	#IsUpload: true
	Target:    string
	Path:      string
}
