package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"

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
		f := filepath.Join(*cacheDir, modulePath, "@v", cachefile())
		if _, err := os.Stat(f); err != nil {
			if !os.IsNotExist(err) {
				log.Fatal(err)
			}
			fmt.Printf("%q does not exist.\n", f)
		}

		srcDir := "/Users/julieqiu/go/pkg/mod/golang.org/x/tools@v0.1.0/godoc"
		load.LegacyCachedImport(&build.Default, pkgPath, srcDir, modulePath, *cacheDir, build.FindOnly)
	}
	// Check for file.
}

func cachefile() string {
	return "cachefile"
}
