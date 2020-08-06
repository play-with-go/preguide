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

	Steps: [string]: [#Language]: #Step

	Defs: [string]: _
}

#Prestep: {
	Package: string
	Args: [...string]
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
