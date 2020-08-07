package embed

//go:generate go run github.com/jteeuwen/go-bindata/go-bindata -pkg embed -o gen_bindata.go -nocompress -prefix ../../ -ignore ".(go|mod|sum|vimrc)$" ../../ ../../out ../../cue.mod
//go:generate go run cuelang.org/go/cmd/cue cmd -t file=gen_bindata.go prefix
//go:generate gofmt -w -s gen_bindata.go
