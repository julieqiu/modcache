package load

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// cachedImport is cfg.BuildContext.Import but cached.
func CachedImport(ctx *build.Context,
	path, srcDir, modulePath, cacheDir string, mode build.ImportMode) (*build.Package, error) {

	// 1. Does there exist a Cache?
	// If so, we are done.
	//
	// If not, nothing is cached so call
	// ctx.ImportDir.
	fmt.Println(1)
	c, err := LoadCache(cacheDir, modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build cache at %s: %s\n", cacheDir+modulePath, err)
	}
	/*
		if c.exists {
			return c.Package()
		}
	*/

	// 2. The cache does not exist.
	// Load the package, then write it to the cache.
	fmt.Println(2)
	pkg, err := ctx.Import(path, srcDir, mode|build.FindOnly)
	if err != nil {
		return nil, err
	}

	fmt.Println(3)
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
	fmt.Println(4)
	cp.FileHash = make(map[string]string)
	fmt.Println("-----------------")
	for _, file := range allFiles {
		sum, err := FileHash(filepath.Join(pkg.Dir, file))
		if err == nil {
			cp.FileHash[file] = hex.EncodeToString(sum[:])
			fmt.Println("-----> ", file, cp.FileHash[file])
		}
	}
	fmt.Println(5)
	data, err := json.MarshalIndent(&cp, "", "\t")
	if err == nil {
		data = append(data, '\n')
		// Write to the cache
		fmt.Println(6)
		fmt.Println(string(data))
		c.PutBytes(actionID(ctx, pkg.Dir, mode), data)
	}
	return pkg, nil
}

func LoadCache(cacheDir, modulePath string) (*Cache, error) {
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

func (c *Cache) Package() (*build.Package, error) {
	fmt.Println("finished")
	return &build.Package{}, nil
}

func uncached(ctx *build.Context, dir string, mode build.ImportMode) (*build.Package, error) {
	return ctx.ImportDir(dir, mode)
}

func actionID(ctx *build.Context, dir string, mode build.ImportMode) ActionID {
	// We have a list of files.
	// Create a new hash.
	h := NewHash("build.Import")
	if debugHash {
		fmt.Fprintf(h, "ImportDir %s mode %d\n", dir, int(mode))
		fmt.Fprintf(h, "cfg goarch %q goos %q goroot %q gopath %q\n",
			ctx.GOARCH, ctx.GOOS, ctx.GOROOT, ctx.GOPATH)
		fmt.Fprintf(h, "cfg cgoenabled %v useallfiles %v compiler %q\n",
			ctx.CgoEnabled, ctx.UseAllFiles, ctx.Compiler)
		fmt.Fprintf(h, "cfg buildtags %q releasetags %q installsuffix %q\n",
			ctx.BuildTags, ctx.ReleaseTags, ctx.InstallSuffix)
		/*
			for _, info := range infos {
				fmt.Fprintf(h, "name %s size %d mtime %d\n", info.Name(), info.Size(), info.ModTime().UnixNano())
				if sys := info.Sys(); sys != nil {
					fmt.Println(h, sys)
				}
			}
		*/
	}
	return h.Sum()
}
