package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("#!/usr/bin/env bash\n")
	fmt.Printf("export GREETING=%v\n", os.Args[1])
}
