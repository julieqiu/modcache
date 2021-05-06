package load

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/julieqiu/modcache/build"
)

// cachedImport is cfg.BuildContext.Import but cached.
func CachedImport(ctx *build.Context,
	path, srcDir, modulePath, cacheDir string, mode build.ImportMode) (*build.Package, error) {

	// 1. Does there exist a Cache?
	// If so, we are done.
	//
	// If not, nothing is cached so call
	// ctx.ImportDir.
	c, err := LoadCache(cacheDir, modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build cache at %s: %s\n", cacheDir+modulePath, err)
	}
	/*
		if c.exists {
			fmt.Println("Found!")
			return c.Package()
		}
	*/

	// 2. The cache does not exist.
	// Load the package, then write it to the cache.
	fmt.Printf("Cache does not exist; importing %q from %q\n\n", path, srcDir)
	pkg, err := ctx.Import(path, srcDir, build.ImportComment)
	if err != nil {
		return nil, err
	}
	spew.Dump(pkg)

	// We have the:
	// - Package
	// - Loaded all the files of that package
	var cp LegacyCachedPackage
	cp.Build = *pkg
	allFiles := StringList(
		pkg.GoFiles,
		pkg.CgoFiles,
		pkg.CFiles,
		pkg.CXXFiles,
		pkg.MFiles,
		pkg.HFiles,
		pkg.FFiles,
		pkg.SFiles,
		pkg.SwigFiles,
		pkg.SwigCXXFiles,
		pkg.SysoFiles,
		pkg.TestGoFiles,
		pkg.XTestGoFiles,
	)
	cp.FileHash = make(map[string]string)
	fmt.Println("Looping through all of the files in the package...")
	for _, file := range allFiles {
		sum, err := FileHash(filepath.Join(pkg.Dir, file))
		if err == nil {
			cp.FileHash[file] = hex.EncodeToString(sum[:])
			fmt.Println("-----> ", file, cp.FileHash[file])
		}
	}
	fmt.Println("Marshaling data to file")
	data, err := json.MarshalIndent(&cp, "", "\t")
	if err == nil {
		data = append(data, '\n')
		// Write to the cache
		fmt.Println("Writing to cache...")
		c.PutBytes(data)
	}
	return pkg, nil
}

func LoadCache(cacheDir, modulePath string) (*Cache, error) {
	fmt.Printf("Loading %q from cache %q\n\n", modulePath, cacheDir)

	dir := filepath.Join(cacheDir, modulePath, "@v")
	c := &Cache{
		dir: dir,
		now: time.Now,
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: fmt.Errorf("not a directory")}
	}
	fn := c.fileName([HashSize]byte{}, "")
	fmt.Println(fn)
	info, err = os.Stat(fn)
	if os.IsNotExist(err) {
		return c, nil
	}

	c.exists = true
	return c, nil
}

func (c *Cache) Package(path, dir string) (*build.Package, error) {
	// Unmarshal file to cache

	/*
			p := &build.Package{
				ImportPath:        path,
				Dir:               dir,
				GoFiles:           c.GoFiles,           // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
				CgoFiles:          c.CgoFiles,          // .go source files that import "C"
				IgnoredGoFiles:    c.IgnoredGoFiles,    // .go source files ignored for this build (including ignored _test.go files)
				InvalidGoFiles:    c.InvalidFiles,      // .go source files with detected problems (parse error, wrong package name, and so on)
				IgnoredOtherFiles: c.IgnoredOtherFiles, // non-.go source files ignored for this build
				CFiles:            c.CFiles,            // .c source files
				CXXFiles:          c.CXXFiles,          // .cc, .cpp and .cxx source files
				MFiles:            c.MFiles,            // .m (Objective-C) source files
				HFiles:            c.HFiles,            // .h, .hh, .hpp and .hxx source files
				FFiles:            c.FFiles,            // .f, .F, .for and .f90 Fortran source files
				SFiles:            c.SFiles,            // .s source files
				SwigFiles:         c.SwigFiles,         // .swig files
				SwigCXXFiles:      c.SwigCXXFiles,      // .swigcxx files
				SysoFiles:         c.SysoFiles,         // .syso system object files to add to archive
				Imports:           []string{},          // import paths from GoFiles, CgoFiles
			}

		if path == "" {
			return p, fmt.Errorf("import %q: invalid import path", path)
		}
	*/
	return &build.Package{}, nil
}

func uncached(ctx *build.Context, dir string, mode build.ImportMode) (*build.Package, error) {
	return ctx.ImportDir(dir, mode)
}
