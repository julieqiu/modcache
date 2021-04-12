package main

import (
	"go/build"
	"log"

	"github.com/julieqiu/cache/load"
)

func main() {
	path := "golang.org/x/image/draw"
	srcDir := "/Users/julieqiu/go/src/golang.org/x/image"

	// Load package.
	// Import always returns bp != nil, even if an error occurs,
	// in order to return partial information.
	//
	// TODO: After Go 1, decide when to pass build.AllowBinary here.
	// See issue 3268 for mistakes to avoid.
	buildMode := build.ImportComment
	_, err := load.CachedImport(&build.Default, path, srcDir, buildMode)
	if err != nil {
		log.Fatal(err)
	}
}
