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
	c, err := LoadCache(cacheDir, modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build cache at %s: %s\n", cacheDir+modulePath, err)
	}
	if c.exists {
		fmt.Println("Found!")
		return c.Package()
	}

	// 2. The cache does not exist.
	// Load the package, then write it to the cache.
	fmt.Printf("Cache does not exist; importing %q from %q\n\n", path, srcDir)
	pkg, err := ctx.Import(path, srcDir, mode|build.FindOnly)
	if err != nil {
		return nil, err
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
	fmt.Println("Looping through all of the files in the package...\n")
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

func (c *Cache) Package() (*build.Package, error) {
	return &build.Package{}, nil
}

func uncached(ctx *build.Context, dir string, mode build.ImportMode) (*build.Package, error) {
	return ctx.ImportDir(dir, mode)
}
