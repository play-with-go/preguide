package main

// CUEDef is the string quoted output of cue def for the current package. This
// constant exists as a workaround until the full intent and capability of
// cuelang.org/go/encoding/gocode/... is established.
const CUEDef = "package preguide\n\n#Guide: {\n\tPresteps: [...#Prestep]\n\n\t// We can't make this conditional on len(Steps) because\n\t// of cuelang.org/issue/279. Hence we validate in code\n\tImage?: string\n\tSteps: [string]: en: #Command | #CommandFile | #Upload | #UploadFile\n\tDefs: {\n\t\t...\n\t}\n}\n#Prestep: {\n\tPackage: string\n\tArgs: [...string]\n}\n#Command: {\n\t#IsCommand: true\n\tSource:     string\n}\n#CommandFile: {\n\t#IsCommandFile: true\n\tPath:           string\n}\n#Upload: {\n\tSource:    string\n\t#IsUpload: true\n\tTarget:    string\n}\n#UploadFile: {\n\tPath:      string\n\t#IsUpload: true\n\tTarget:    string\n}\n"
