package load

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/julieqiu/modcache/build"
	"github.com/julieqiu/modcache/godoc"
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

	// We have the:
	// - Package
	// - Loaded all the files of that package
	goFiles := StringList(
		pkg.GoFiles,
		pkg.TestGoFiles,
		pkg.XTestGoFiles,
	)
	fmt.Println("Looping through all of the files in the package...")

	if c.Dirs == nil {
		c.Dirs = map[string]*Dir{}
	}
	d := &Dir{
		Path:          pkg.Dir,
		Name:          pkg.Name,
		ImportComment: pkg.ImportComment,
		Doc:           pkg.Doc,
		ImportPath:    pkg.ImportPath,
		Root:          pkg.Root,
		SrcRoot:       pkg.SrcRoot,
		PkgRoot:       pkg.PkgRoot,
	}
	c.Dirs[pkg.Dir] = d
	for _, file := range goFiles {
		fset := token.NewFileSet()
		info := &build.FileInfo{Fset: fset, Name: filepath.Join(pkg.Dir, file)}
		f, err := os.Open(filepath.Join(pkg.Dir, file))
		if err != nil {
			return nil, err
		}
		if err := build.ReadGoInfo(f, info); err != nil {
			return nil, err
		}
		f.Close()

		fi := &FileInfo{Name: info.Name}
		for _, imp := range info.Imports {
			fi.Imports = append(fi.Imports, FileImport{Path: imp.Path, Pos: imp.Pos})
		}
		for _, emb := range info.Embeds {
			fi.Embeds = append(fi.Embeds, FileEmbed{Pattern: emb.Pattern, Pos: emb.Pos})
		}

		d.GoFiles = append(d.GoFiles, fi)
		syms, err := loadIdentifiers(pkg.ImportPath, filepath.Join(pkg.Dir, file))
		if err != nil {
			return nil, err
		}
		fi.Exports = syms
		fi.BuildTags, err = builds(info.Header)
		if err != nil {
			return nil, err
		}
	}

	nonGoFiles := StringList(
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
	)
	for _, file := range nonGoFiles {
		d.NonGoFiles = append(d.NonGoFiles, file)
	}

	fmt.Println("Marshaling data to file")
	data, err := json.MarshalIndent(&c.Dirs, "", "\t")
	if err == nil {
		data = append(data, '\n')
		// Write to the cache
		fmt.Println("Writing to cache...")
		c.PutBytes(data)
	}
	return pkg, nil
}

func loadIdentifiers(importPath, filename string) ([]string, error) {
	fset := token.NewFileSet()
	a, err := mustParse(fset, filename)
	if err != nil {
		return nil, err
	}
	p, err := doc.NewFromFiles(fset, []*ast.File{a}, importPath)
	if err != nil {
		return nil, err
	}
	return godoc.GetSymbols(p)
}

func mustParse(fset *token.FileSet, filename string) (*ast.File, error) {
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return f, nil
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

var (
	slashSlash  = []byte("//")
	bSlashSlash = []byte(slashSlash)
)

func builds(content []byte) (_ []string, err error) {
	// Pass 1. Identify leading run of // comments and blank lines,
	// which must be followed by a blank line.
	// Also identify any //go:build comments.
	content, _, _, err = build.ParseFileHeader(content)
	if err != nil {
		return nil, err
	}

	// Pass 2.  Process each +build line in the run.
	p := content
	var builds []string
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, bSlashSlash) {
			continue
		}
		line = bytes.TrimSpace(line[len(bSlashSlash):])
		if len(line) > 0 && line[0] == '+' {
			// Looks like a comment +line.
			f := strings.Fields(string(line))
			if f[0] == "+build" {
				for _, tok := range f[1:] {
					builds = append(builds, tok)
				}
			}
		}
	}
	return builds, nil
}
