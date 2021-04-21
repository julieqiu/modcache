package load

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

// cachedPackage is the data structure stored in the cache.
// It would be more efficient not to use JSON here.
type LegacyCachedPackage struct {
	// TODO: what is https://pkg.go.dev/go/token#Position
	Build    build.Package
	FileHash map[string]string
}

// cachedImport is cfg.BuildContext.Import but cached.
func LegacyCachedImport(ctx *build.Context, path, srcDir, modulePath, cacheDir string, mode build.ImportMode) (*build.Package, error) {
	// Rewrite Import into ImportDir by asking Import
	// to find the dir but not read any files.
	// Then we don't need to have separate cache entries for search srcDir.
	fmt.Println("ctx.Import: ", path, srcDir)
	// TODO: why does this return
	// /Users/julieqiu/go/pkg/mod/golang.org/x/tools@v0.0.0-20200915173823-2db8f0ff891c/godoc
	p, err := ctx.Import(path, srcDir, mode|build.FindOnly)
	if err != nil {
		fmt.Println(1)
		return p, err
	}
	/*
		if mode&build.FindOnly != 0 {
			fmt.Println(2)
			return p, nil
		}
	*/
	// The IgnoreVendor bit doesn't matter to ImportDir.
	// Clear it to get more cache hits.
	fmt.Println(3)
	return legacyCachedImportDir(ctx, p.Dir, modulePath, cacheDir, mode&^build.IgnoreVendor)
}

var cacheVerify = os.Getenv("GOCMDCACHEVERIFY") == "1"

const HashSize = 32

func legacyCachedImportDir(ctx *build.Context, dir, modulePath, cacheDir string, mode build.ImportMode) (*build.Package, error) {
	uncached := func() (*build.Package, error) {
		fmt.Println("uncached: ctx.ImportDir")
		return ctx.ImportDir(dir, mode)
	}
	// 1. Does there exist a Cache? If not, nothing is cached so call
	// ctx.ImportDir.
	c := ModCache(cacheDir, modulePath)
	// spew.Dump(c)
	if c == nil {
		println("NO CACHE")
		fmt.Println(1)
		return uncached()
	}
	fmt.Println("cachedImportDir: dir: ", dir)

	// 2. A Cache exists and we know the directory we should read from.
	//
	// Read infos ([]fs.FileInfo) from that dir:
	// /Users/julieqiu/go/pkg/mod/golang.org/x/tools@v0.1.0/godoc
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		println("BAD READDIR", err.Error())
		fmt.Println(2)
		return uncached()
	}
	// spew.Dump(infos)

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
		for _, info := range infos {
			fmt.Fprintf(h, "name %s size %d mtime %d\n", info.Name(), info.Size(), info.ModTime().UnixNano())
			if sys := info.Sys(); sys != nil {
				fmt.Println(h, sys)
			}
		}
	}
	actionID := h.Sum()
	fmt.Println("actionID")
	// spew.Dump(actionID)

	fmt.Println("-----------------")
	// actionID = hash
	// 1. GetBytes of the actionID
	//		GetBytes looks up the action ID in the cache and returns
	//		the corresponding output bytes.
	//		GetBytes should only be used for data that can be expected to fit
	//		in memory.
	//
	// 2. Unmarshal the data from those bytes into the cached package
	// 3. For every filename, filehash:
	//		Decode the string
	//		(?) SetFileHash sets the hash returned by FileHash for file.
	var cacheEntry []byte
	if data, _, err := c.GetBytes(actionID); err == nil {
		if cacheVerify {
			cacheEntry = data
		} else {
			var cp LegacyCachedPackage
			if err := json.Unmarshal(data, &cp); err == nil {
				for name, hash := range cp.FileHash {
					fmt.Println(name, hash)
					var sum [HashSize]byte
					x, err := hex.DecodeString(hash)
					if err == nil && len(x) == HashSize {
						copy(sum[:], x)
						SetFileHash(filepath.Join(dir, name), sum)
					}
				}
				return &cp.Build, nil
			}

		}
	}
	// TODO: why call uncached here?
	fmt.Println("-----------------")
	pkg, err := uncached()
	if err != nil {
		return pkg, err
	}
	// Got something again, call log.Fatal if something went wrong?
	if pkg.Dir != dir {
		log.Fatalf("internal error: LoadImport: found %s but expected %s", pkg.Dir, dir)
	}

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
	fmt.Println("-----------------")
	for _, file := range allFiles {
		sum, err := FileHash(filepath.Join(dir, file))
		if err == nil {
			cp.FileHash[file] = hex.EncodeToString(sum[:])
			fmt.Println("-----> ", file, cp.FileHash[file])
		}
	}
	data, err := json.MarshalIndent(&cp, "", "\t")
	if err == nil {
		data = append(data, '\n')
		if cacheEntry != nil && !bytes.Equal(data, cacheEntry) {
			fmt.Fprintf(os.Stderr, "cfg goarch %q goos %q goroot %q gopath %q cgoenabled %v useallfiles %v compiler %q buildtags %q releasetags %q installsuffix %q\n",
				ctx.GOARCH, ctx.GOOS, ctx.GOROOT, ctx.GOPATH, ctx.CgoEnabled, ctx.UseAllFiles, ctx.Compiler, ctx.BuildTags, ctx.ReleaseTags, ctx.InstallSuffix)
			fmt.Fprintf(os.Stderr, "cache mismatch for %s:\nFOUND:\n%s\nCOMPUTED:\n%s", dir, cacheEntry, data)
			panic(1)
		}
		// Write to the cache
		c.PutBytes(actionID, data)
	}

	// Return the package
	return pkg, err
}
