package out

// CUEDef is the string quoted output of cue def for the current package. This
// constant exists as a workaround until the full intent and capability of
// cuelang.org/go/encoding/gocode/... is established.
const CUEDef = "package out\n\n#GuideOutput: {\n\tPresteps: [...#Prestep]\n\tImage: string\n\tLangs: #Langs\n\tDefs: [string]: _\n}\n\n#Prestep: {\n\tPackage: string\n\tVersion: string\n\tBuildID: string\n\tArgs: [...string]\n}\n\n#Langs: {\n\ten: #LangSteps\n}\n\n#LangSteps: {\n\tHash:  string\n\tSteps: #Steps\n}\n\n#Steps: {\n\t[string]: #CommandStep | #UploadStep\n}\n\n#CommandStep: {\n\tName:  string\n\tOrder: int\n\tStmts: [...#Stmt]\n}\n\n#Stmt: {\n\tNegated:  bool\n\tCmdStr:   string\n\tExitCode: int\n\tOutput:   string\n}\n\n#UploadStep: {\n\tName:   string\n\tOrder:  int\n\tSource: string\n\tTarget: string\n}\n"
