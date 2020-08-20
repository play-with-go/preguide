package preguide

import "list"

// TODO: keep this in sync with the Go definitions
#StepType: int

#StepTypeCommand:     #StepType & 1
#StepTypeCommandFile: #StepType & 2
#StepTypeUpload:      #StepType & 3
#StepTypeUploadFile:  #StepType & 4

#Guide: {

	#Step: (#Command | #CommandFile | #Upload | #UploadFile ) & _#stepCommon

	_#stepCommon: {
		Name:     string
		StepType: #StepType
		Terminal: string
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

	#Terminal: {
		// Image is the Docker image that will be used for the terminal session
		Name:  string
		Image: string
	}

	Presteps: [...#Prestep]

	// Delims are the delimiters used in the guide prose and steps
	// for environment variable substitution. A template substitution
	// of the environment variable ABC therefore looks like "{{ .ABC }}"
	Delims: *["{{", "}}"] | [string, string]

	Steps: [string]: [#Language]: #Step

	Steps: [name=string]: [#Language]: {
		// TODO: remove post upgrade to latest CUE? Because at that point
		// the defaulting in #TerminalName will work
		Terminal: *#TerminalNames[0] | string

		Name: name
	}

	// TODO: remove post upgrade to latest CUE? Because at that point
	// the use of or() will work, which will give a better error message
	#TerminalNames: [ for k, _ in Terminals {k}]
	#ok: true & and([ for s in Steps {list.Contains(#TerminalNames, s.en.Terminal)}])

	// Terminals defines the required remote VMs for a given guide
	Terminals: [string]: #Terminal

	Terminals: [name=string]: {
		Name: name
	}

	Defs: [string]: _
}

#Prestep: {
	Package: string
	Path:    *"/" | string
	Args:    *null | _
}

// TODO: keep in sync with Go code
#Language: "ab" | "aa" | "af" | "ak" | "sq" | "am" | "ar" | "an" | "hy" | "as" | "av" | "ae" | "ay" | "az" | "bm" | "ba" | "eu" | "be" | "bn" |
	"bh" | "bi" | "bs" | "br" | "bg" | "my" | "ca" | "ch" | "ce" | "ny" | "zy" | "cv" | "kw" | "co" | "cr" | "hr" | "cs" | "da" | "dv" |
	"nl" | "dz" | "en" | "eo" | "et" | "ee" | "fo" | "fj" | "fi" | "fr" | "ff" | "gl" | "ka" | "de" | "el" | "gn" | "gu" | "ht" | "ha" |
	"he" | "hz" | "hi" | "ho" | "hu" | "ia" | "id" | "ie" | "ga" | "ig" | "ik" | "io" | "is" | "it" | "iu" | "ja" | "jv" | "kl" | "kn" |
	"kr" | "ks" | "kk" | "km" | "ki" | "rw" | "ky" | "kv" | "kg" | "ko" | "ku" | "kj" | "la" | "lb" | "lg" | "li" | "ln" | "lo" | "lt" |
	"lu" | "lv" | "gv" | "mk" | "mg" | "ms" | "ml" | "mt" | "mi" | "mr" | "mh" | "mn" | "na" | "nv" | "nd" | "ne" | "ng" | "nb" | "nn" |
	"no" | "ii" | "nr" | "oc" | "oj" | "cu" | "om" | "or" | "os" | "pa" | "pi" | "fa" | "pl" | "ps" | "pt" | "qu" | "rm" | "rn" | "ro" |
	"ru" | "sa" | "sc" | "sd" | "se" | "sm" | "sg" | "sr" | "gd" | "sn" | "si" | "sk" | "sl" | "so" | "st" | "es" | "su" | "sw" | "ss" |
	"sv" | "ta" | "te" | "tg" | "th" | "ti" | "bo" | "tk" | "tl" | "tn" | "to" | "tr" | "ts" | "tt" | "tw" | "ty" | "ug" | "uk" | "ur" |
	"uz" | "ve" | "vi" | "vo" | "wa" | "cy" | "wo" | "fy" | "xh" | "yi" | "yo" | "za" | "zu"

// The following definitions necessarily reference the nested definitions
// in #Guide, because those definitions rely on references to Terminals
// which only makes sense in the context of a #Guide instance

#Step:        #Guide.#Step
#Command:     #Guide.#Command
#CommandFile: #Guide.#CommandFile
#Upload:      #Guide.#Upload
#UploadFile:  #Guide.#UploadFile

// #PrestepServiceConfig is a mapping from prestep package path to endpoint
// configuration.
#PrestepServiceConfig: [string]: #PrestepConfig

// #PrestepConfig is the endpoint configuration for a prestep
#PrestepConfig: {
	Endpoint: string

	// Env defines the list of docker environment values (values that can be
	// passed to docker run's -e flag) that are passed when a prestep is run
	// via docker
	Env: [...string]

	// Env defines the list of docker networks that should be joined when
	// a prestep is run via docker
	Networks: [...string]
}
