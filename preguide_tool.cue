package preguide

import (
	"tool/file"
	"tool/os"
	"strconv"
)

exportCUEDef: *false | bool @tag(export,type=bool)
#CUEDefName:  string
if exportCUEDef == true {
	#CUEDefName: "CUEDef"
}
if exportCUEDef == false {
	#CUEDefName: "cueDef"
}

embedFile: string @tag(embed)

command: embed: {
	gen: file.Read & {
		filename: embedFile
		contents: string
	}

	pkg: os.Getenv & {
		GOPACKAGE: string
	}

	embed: file.Create & {
		filename: "gen_cue.go"
		contents: """
		package \(pkg.GOPACKAGE)

		// \(#CUEDefName) is the string quoted output of cue def for the current package. This
		// constant exists as a workaround until the full intent and capability of
		// cuelang.org/go/encoding/gocode/... is established.
		const \(#CUEDefName) = \(strconv.Quote(gen.contents))

		"""
	}
}
