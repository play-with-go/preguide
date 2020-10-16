// Code generated by go-bindata. DO NOT EDIT.

package embed

import (
	"fmt"
	"strings"
)

var _cue_mod_module_cue = []byte(`module: "github.com/play-with-go/preguide"
`)

func cue_mod_module_cue() ([]byte, error) {
	return _cue_mod_module_cue, nil
}

var _out_out_cue = []byte(`package out

import "github.com/play-with-go/preguide"

#GuideOutput: {
	Delims: [string, string]
	Presteps: [...#Prestep]
	Terminals: [...preguide.#Terminal]
	Scenarios: [...preguide.#Scenario]
	Langs: [preguide.#Language]: #LangSteps
	Defs: [string]:              _
	Networks: [...string]
	Env: [...string]
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

	// Variables is the set of environment variables that resulted
	// from the execution of the prestep
	Variables: [...string]
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

	// TrimmedOutput is the Output from the statement
	// with the trailing \n removed
	TrimmedOutput: string
}

#UploadStep: {
	_#stepCommon
	Renderer: preguide.#Renderer
	Language: string
	Source:   string
	Target:   string
}

// GuideStructures maps a guide name to its #GuideStructure
#GuideStructures: [string]: #GuideStructure

// A #GuideStructure defines the prestep and terminal
// structure of a guide. Note there is some overlap here
// with the #GuideOutput type above... perhaps we can
// conslidate at some point. The main difference is that
// GuideStructure is a function of the input types.
#GuideStructure: {
	Delims: [string, string]
	Presteps: [...preguide.#Prestep]
	Terminals: [...preguide.#Terminal]
	Scenarios: [...preguide.#Scenario]
	Networks: [...string]
	Env: [...string]
}
`)

func out_out_cue() ([]byte, error) {
	return _out_out_cue, nil
}

var _preguide_cue = []byte(`package preguide

import (
	"list"
	"path"
	"regexp"
)

// TODO: keep this in sync with the Go definitions
#StepType: int

#StepTypeCommand:     #StepType & 1
#StepTypeCommandFile: #StepType & 2
#StepTypeUpload:      #StepType & 3
#StepTypeUploadFile:  #StepType & 4

#Guide: {

	#Step: (#Command | #CommandFile | #Upload | #UploadFile ) & {
		Name:     string
		StepType: #StepType
		Terminal: string
	}

	// Change this to a hidden definition once cuelang.org/issue/533 is resolved
	#stepCommon: {
		Name:     string
		StepType: #StepType
		Terminal: string
	}

	#uploadCommon: {
		Target: string

		// The language of the content being uploaded, e.g. go
		// This gets used on the markdown code block, hence the
		// values supported here are a function of the markdown
		// parse on the other end.
		Language: *regexp.FindSubmatch("^.(.*)", path.Ext(Target))[1] | string

		// Renderer defines how the upload file contents will be
		// rendered to the user in the guide.
		Renderer: #Renderer
	}

	#Command: {
		#stepCommon
		StepType: #StepTypeCommand
		Source:   string
	}

	#CommandFile: {
		#stepCommon
		StepType: #StepTypeCommandFile
		Path:     string
	}

	#Upload: {
		#stepCommon
		#uploadCommon
		StepType: #StepTypeUpload
		Source:   string
	}

	#UploadFile: {
		#stepCommon
		#uploadCommon
		StepType: #StepTypeUploadFile
		Path:     string
	}

	// Networks is the list of docker networks to connect to when running
	// this guide.
	Networks: [...string]

	// Env is the environment to pass to docker containers when running
	// this guide.
	Env: [...string]

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

	// Scenarios define the various images under which this guide will be
	// run
	Scenarios: [string]: #Scenario
	Scenarios: [name=string]: {
		Name: name
	}

	_#ScenarioName: or([ for name, _ in Scenarios {name}])

	for scenario, _ in Scenarios for terminal, _ in Terminals {
		Terminals: "\(terminal)": Scenarios: "\(scenario)": #TerminalScenario
	}

	// TODO: remove post upgrade to latest CUE? Because at that point
	// the use of or() will work, which will give a better error message
	#TerminalNames: [ for k, _ in Terminals {k}]
	#ok: true & and([ for s in Steps for l in s {list.Contains(#TerminalNames, l.Terminal)}])

	// Terminals defines the required remote VMs for a given guide
	Terminals: [string]: #Terminal

	Terminals: [name=string]: {
		Name: name
	}

	Defs: [string]: _
}

#Terminal: {
	Name:        string
	Description: string
	Scenarios: [string]: #TerminalScenario
}

#TerminalScenario: {
	Image: string
}

#Scenario: {
	Name:        string
	Description: string
}

#Prestep: {
	Package: string
	Path:    *"/" | string
	Args?:   _
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

	// Networks defines the list of docker networks to connect to when
	// running this prestep.
	Networks: [...string]

	// Env is the environment to pass to docker containers when running
	// this prestep.
	Env: [...string]
}

// Renderers define what part (or whole) of an upload file should be shown (rendered)
// to the user in the guide.
#Renderer: (*#RenderFull | #RenderLineRanges | #RenderDiff) & _#rendererCommon

#RendererType: int

#RendererTypeFull:       #RendererType & 1
#RendererTypeLineRanges: #RendererType & 2
#RendererTypeDiff:       #RendererType & 3

_#rendererCommon: {
	RendererType: #RendererType
	...
}

#RenderFull: {
	_#rendererCommon
	RendererType: #RendererTypeFull
}

#RenderLineRanges: {
	_#rendererCommon
	RendererType: #RendererTypeLineRanges
	Ellipsis:     *"..." | string
	Lines: [...[int, int]]
}

#RenderDiff: {
	_#rendererCommon
	RendererType: #RendererTypeDiff
	Pre:          string
}

// Post upgrade to latest CUE we have a number of things to change/test, with /
// reference to https://gist.github.com/myitcv/399ed50f792b49ae7224ee5cb3e504fa#file-304b02e-cue
//
// 1. Move to the use of #TerminalName (probably hidden) as a type for a terminal's
// name in #stepCommon
// 2. Try and move to the advanced definition of Steps: [string]: [lang] to be the
// disjunction of #Step or [scenario]: #Step
// 3. Ensure that a step's name can be defaulted for this advanced definition (i.e.
// that if a step is specified at the language level its name defaults, but also
// if it is specified at the scenario level)
`)

func preguide_cue() ([]byte, error) {
	return _preguide_cue, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() ([]byte, error){
	"cue.mod/module.cue": cue_mod_module_cue,
	"out/out.cue":        out_out_cue,
	"preguide.cue":       preguide_cue,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for name := range node.Children {
		rv = append(rv, name)
	}
	return rv, nil
}

type _bintree_t struct {
	Func     func() ([]byte, error)
	Children map[string]*_bintree_t
}

var _bintree = &_bintree_t{nil, map[string]*_bintree_t{
	"cue.mod": {nil, map[string]*_bintree_t{
		"module.cue": {cue_mod_module_cue, map[string]*_bintree_t{}},
	}},
	"out": {nil, map[string]*_bintree_t{
		"out.cue": {out_out_cue, map[string]*_bintree_t{}},
	}},
	"preguide.cue": {preguide_cue, map[string]*_bintree_t{}},
}}
