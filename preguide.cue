package preguide

#Guide: {
	Image: string
	Steps: [string]: en: #Command | #CommandFile | #Upload | #UploadFile
}

#Command: {
	#IsCommand: true
	Source: string
}

#CommandFile: {
	#IsCommandFile: true
	Path: string
}

#Upload: {
	#IsUpload: true
	Target: string
	Source: string
}

#UploadFile: {
	#IsUpload: true
	Target: string
	Path: string
}
