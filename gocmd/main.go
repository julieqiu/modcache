package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/julieqiu/modcache/build"
	"github.com/julieqiu/modcache/load"
)

var (
	q        = flag.Bool("q", false, "")
	cacheDir = flag.String("cache", "/Users/julieqiu/go/pkg/mod/cache/download", "")
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, `
gocmd [module path] [import path]
gocmd -q [name] [symbol]
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}
	args := flag.Args()
	if *q {
		name := args[0]
		symbol := args[1]
		fmt.Println(name, symbol)
		return
	} else {
		modulePath := args[0]
		pkgPath := args[1]
		if !strings.HasPrefix(pkgPath, modulePath) {
			flag.Usage()
			log.Fatalf("The specified import path must be a package in the module.")
		}
		srcDir := "/Users/julieqiu/go/pkg/mod/golang.org/x/tools@v0.1.0/godoc"
		load.CachedImport(&build.Default, pkgPath, srcDir, modulePath, *cacheDir, build.FindOnly)
	}
	// Check for file.
}
