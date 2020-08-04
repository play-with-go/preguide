package main

// cueDef is the string quoted output of cue def for the current package. This
// constant exists as a workaround until the full intent and capability of
// cuelang.org/go/encoding/gocode/... is established.
const cueDef = "package preguide\n\n#Guide: {\n\tPresteps: [...#Prestep]\n\n\t// Delims are the delimiters used in the guide prose and steps\n\t// for environment variable substitution. A template substitution\n\t// of the environment variable ABC therefore looks like \"{{ .ABC }}\"\n\tDelims: *[\"{{\", \"}}\"] | [string, string]\n\n\t// Images are optional because a guide does not need to have\n\t// any steps. However, we can't make this conditional on len(Steps) because\n\t// of cuelang.org/issue/279. Hence we validate in code.\n\tImage?: string\n\tSteps: [string]: en: (#Command | #CommandFile | #Upload | #UploadFile) & {\n\t\tStepType: #StepType\n\t}\n\tDefs: {\n\t\t...\n\t}\n}\n#Prestep: {\n\tPackage: string\n\tArgs: [...string]\n}\n#Command: {\n\tStepType: 1\n\tSource:   string\n}\n#CommandFile: {\n\tStepType: 2\n\tPath:     string\n}\n#Upload: {\n\tStepType: 3\n\tSource:   string\n\tTarget:   string\n}\n#UploadFile: {\n\tStepType: 4\n\tPath:     string\n\tTarget:   string\n}\n#StepType:            int\n#StepTypeCommand:     1\n#StepTypeCommandFile: 2\n#StepTypeUpload:      3\n#StepTypeUploadFile:  4\n"
