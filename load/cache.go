package load

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StringList flattens its arguments into a single []string.
// Each argument in args must have type string or []string.
func StringList(args ...interface{}) []string {
	var x []string
	for _, arg := range args {
		switch arg := arg.(type) {
		case []string:
			x = append(x, arg...)
		case string:
			x = append(x, arg)
		default:
			panic("stringList: invalid argument of type " + fmt.Sprintf("%T", arg))
		}
	}
	return x
}

// A Cache is a package cache, backed by a file system directory tree.
type Cache struct {
	dir    string
	now    func() time.Time
	exists bool
}

var (
	defaultOnce  sync.Once
	defaultCache *Cache
)

func ModCache(cacheDir, modulePath string) *Cache {
	dir := filepath.Join(cacheDir, modulePath, "@v")
	c, err := Open(dir)
	if err != nil {
		log.Fatalf("failed to initialize build cache at %s: %s\n", dir, err)
	}
	return c
}

// Default returns the default cache to use, or nil if no cache should be used.
func Default() *Cache {
	defaultOnce.Do(initDefaultCache)
	return defaultCache
}

// initDefaultCache does the work of finding the default cache
// the first time Default is called.
func initDefaultCache() {
	dir := DefaultDir()
	if dir == "off" {
		if defaultDirErr != nil {
			log.Fatalf("build cache is required, but could not be located: %v", defaultDirErr)
		}
		log.Fatalf("build cache is disabled by GOCACHE=off, but required as of Go 1.12")
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		log.Fatalf("failed to initialize build cache at %s: %s\n", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README")); err != nil {
		// Best effort.
		os.WriteFile(filepath.Join(dir, "README"), []byte(cacheREADME), 0666)
	}
	c, err := Open(dir)
	if err != nil {
		log.Fatalf("failed to initialize build cache at %s: %s\n", dir, err)
	}
	defaultCache = c
}

// cacheREADME is a message stored in a README in the cache directory.
// Because the cache lives outside the normal Go trees, we leave the
// README as a courtesy to explain where it came from.
const cacheREADME = `This directory holds cached build artifacts from the Go build system.
Run "go clean -cache" if the directory is getting too large.
See golang.org to learn more about Go.`

// hashSalt is a salt string added to the beginning of every hash
// created by NewHash. Using the Go version makes sure that different
// versions of the go command (or even different Git commits during
// work on the development branch) do not address the same cache
// entries, so that a bug in one version does not affect the execution
// of other versions. This salt will result in additional ActionID files
// in the cache, but not additional copies of the large output files,
// which are still addressed by unsalted SHA256.
//
// We strip any GOEXPERIMENTs the go tool was built with from this
// version string on the assumption that they shouldn't affect go tool
// execution. This also allows bootstrapping to converge faster
// because dist builds go_bootstrap without any experiments.
var hashSalt = []byte(stripExperiment(runtime.Version()))

// NewHash returns a new Hash.
// The caller is expected to Write data to it and then call Sum.
func NewHash(name string) *Hash {
	h := &Hash{h: sha256.New(), name: name}
	// fmt.Fprintf(os.Stderr, "HASH[%s]\n", h.name)
	h.Write(hashSalt)
	h.buf = new(bytes.Buffer)
	return h
}

// GetBytes looks up the action ID in the cache and returns
// the corresponding output bytes.
// GetBytes should only be used for data that can be expected to fit in memory.
func (c *Cache) GetBytes(id ActionID) ([]byte, Entry, error) {
	entry, err := c.Get(id)
	if err != nil {
		return nil, entry, err
	}
	fmt.Println(c.OutputFile(entry.OutputID))
	data, _ := os.ReadFile(c.OutputFile(entry.OutputID))
	if sha256.Sum256(data) != entry.OutputID {
		return nil, entry, &entryNotFoundError{Err: errors.New("bad checksum")}
	}
	return data, entry, nil
}

// SetFileHash sets the hash returned by FileHash for file.
func SetFileHash(file string, sum [HashSize]byte) {
	hashFileCache.Lock()
	if hashFileCache.m == nil {
		hashFileCache.m = make(map[string][HashSize]byte)
	}
	hashFileCache.m[file] = sum
	hashFileCache.Unlock()
}

// FileHash returns the hash of the named file.
// It caches repeated lookups for a given file,
// and the cache entry for a file can be initialized
// using SetFileHash.
// The hash used by FileHash is not the same as
// the hash used by NewHash.
func FileHash(file string) ([HashSize]byte, error) {
	hashFileCache.Lock()
	out, ok := hashFileCache.m[file]
	hashFileCache.Unlock()
	if ok {
		return out, nil
	}
	h := sha256.New()
	f, err := os.Open(file)
	if err != nil {
		if debugHash {
			fmt.Fprintf(os.Stderr, "HASH %s: %v\n", file, err)
		}
		return [HashSize]byte{}, err
	}
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		if debugHash {
			fmt.Fprintf(os.Stderr, "HASH %s: %v\n", file, err)
		}
		return [HashSize]byte{}, err
	}
	h.Sum(out[:0])
	if debugHash {
		fmt.Fprintf(os.Stderr, "HASH %s: %x\n", file, out)
	}
	SetFileHash(file, out)
	return out, nil
}

func (c *Cache) PutBytes(data []byte) error {
	id := ActionID{} // id doesn't matter
	_, _, err := c.Put(id, bytes.NewReader(data))
	return err
}

// LegacyPutBytes stores the given bytes in the cache as the output for the action ID.
func (c *Cache) LegacyPutBytes(id ActionID, data []byte) error {
	_, _, err := c.Put(id, bytes.NewReader(data))
	return err
}

var (
	defaultDirOnce sync.Once
	defaultDir     string
	defaultDirErr  error
)

// DefaultDir returns the effective GOCACHE setting.
// It returns "off" if the cache is disabled.
func DefaultDir() string {
	// Save the result of the first call to DefaultDir for later use in
	// initDefaultCache. cmd/go/main.go explicitly sets GOCACHE so that
	// subprocesses will inherit it, but that means initDefaultCache can't
	// otherwise distinguish between an explicit "off" and a UserCacheDir error.
	defaultDirOnce.Do(func() {
		defaultDir = os.Getenv("GOCACHE")
		if filepath.IsAbs(defaultDir) || defaultDir == "off" {
			return
		}
		if defaultDir != "" {
			defaultDir = "off"
			defaultDirErr = fmt.Errorf("GOCACHE is not an absolute path")
			return
		}
		// Compute default location.
		dir, err := os.UserCacheDir()
		if err != nil {
			defaultDir = "off"
			defaultDirErr = fmt.Errorf("GOCACHE is not defined and %v", err)
			return
		}
		defaultDir = filepath.Join(dir, "go-build")
	})
	return defaultDir
}

// Open opens and returns the cache in the given directory.
//
// It is safe for multiple processes on a single machine to use the
// same cache directory in a local file system simultaneously.
// They will coordinate using operating system file locks and may
// duplicate effort but will not corrupt the cache.
//
// However, it is NOT safe for multiple processes on different machines
// to share a cache directory (for example, if the directory were stored
// in a network file system). File locking is notoriously unreliable in
// network file systems and may not suffice to protect the cache.
//
func Open(dir string) (*Cache, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: fmt.Errorf("not a directory")}
	}
	//	NOTE: commenting out this line prevents things from being created.
	for i := 0; i < 256; i++ {
		name := filepath.Join(dir, fmt.Sprintf("%02x", i))
		if err := os.MkdirAll(name, 0777); err != nil {
			return nil, err
		}
	}
	c := &Cache{
		dir: dir,
		now: time.Now,
	}
	return c, nil
}

// A Hash provides access to the canonical hash function used to index the cache.
// The current implementation uses salted SHA256, but clients must not assume this.
type Hash struct {
	h    hash.Hash
	name string        // for debugging
	buf  *bytes.Buffer // for verify
}

// Write writes data to the running hash.
func (h *Hash) Write(b []byte) (int, error) {
	if debugHash {
		fmt.Fprintf(os.Stderr, "HASH[%s]: %q\n", h.name, b)
	}
	if h.buf != nil {
		h.buf.Write(b)
	}
	return h.h.Write(b)
}

// Sum returns the hash of the data written previously.
func (h *Hash) Sum() [HashSize]byte {
	var out [HashSize]byte
	h.h.Sum(out[:0])
	if debugHash {
		fmt.Fprintf(os.Stderr, "HASH[%s]: %x\n", h.name, out)
	}
	if h.buf != nil {
		hashDebug.Lock()
		if hashDebug.m == nil {
			hashDebug.m = make(map[[HashSize]byte]string)
		}
		hashDebug.m[out] = h.buf.String()
		hashDebug.Unlock()
	}
	return out
}

// An ActionID is a cache action key, the hash of a complete description of a
// repeatable computation (command line, environment variables,
// input file contents, executable contents).
type ActionID [HashSize]byte

type Entry struct {
	OutputID OutputID
	Size     int64
	Time     time.Time
}

// An OutputID is a cache output key, the hash of an output of a computation.
type OutputID [HashSize]byte

// stripExperiment strips any GOEXPERIMENT configuration from the Go
// version string.
func stripExperiment(version string) string {
	if i := strings.Index(version, " X:"); i >= 0 {
		return version[:i]
	}
	return version
}

var hashFileCache struct {
	sync.Mutex
	m map[string][HashSize]byte
}

var debugHash = false // set when GODEBUG=gocachehash=1

// Get looks up the action ID in the cache,
// returning the corresponding output ID and file size, if any.
// Note that finding an output ID does not guarantee that the
// saved file for that output ID is still available.
func (c *Cache) Get(id ActionID) (Entry, error) {
	// if verify {
	//	return Entry{}, &entryNotFoundError{Err: errVerifyMode}
	// }
	return c.get(id)
}

// OutputFile returns the name of the cache file storing output with the given OutputID.
func (c *Cache) OutputFile(out OutputID) string {
	file := c.fileName(out, "d")
	c.used(file)
	return file
}

// Put stores the given output in the cache as the output for the action ID.
// It may read file twice. The content of file must not change between the two passes.
func (c *Cache) Put(id ActionID, file io.ReadSeeker) (OutputID, int64, error) {
	return c.put(id, file, true)
}

// In GODEBUG=gocacheverify=1 mode,
// hashDebug holds the input to every computed hash ID,
// so that we can work backward from the ID involved in a
// cache entry mismatch to a description of what should be there.
var hashDebug struct {
	sync.Mutex
	m map[[HashSize]byte]string
}

// An entryNotFoundError indicates that a cache entry was not found, with an
// optional underlying reason.
type entryNotFoundError struct {
	Err error
}

func (e *entryNotFoundError) Error() string {
	if e.Err == nil {
		return "cache entry not found"
	}
	return fmt.Sprintf("cache entry not found: %v", e.Err)
}

// get is Get but does not respect verify mode, so that Put can use it.
func (c *Cache) get(id ActionID) (Entry, error) {
	missing := func(reason error) (Entry, error) {
		return Entry{}, &entryNotFoundError{Err: reason}
	}
	f, err := os.Open(c.fileName(id, "a"))
	if err != nil {
		return missing(err)
	}
	defer f.Close()
	entry := make([]byte, entrySize+1) // +1 to detect whether f is too long
	if n, err := io.ReadFull(f, entry); n > entrySize {
		return missing(errors.New("too long"))
	} else if err != io.ErrUnexpectedEOF {
		if err == io.EOF {
			return missing(errors.New("file is empty"))
		}
		return missing(err)
	} else if n < entrySize {
		return missing(errors.New("entry file incomplete"))
	}
	if entry[0] != 'v' || entry[1] != '1' || entry[2] != ' ' || entry[3+hexSize] != ' ' || entry[3+hexSize+1+hexSize] != ' ' || entry[3+hexSize+1+hexSize+1+20] != ' ' || entry[entrySize-1] != '\n' {
		return missing(errors.New("invalid header"))
	}
	eid, entry := entry[3:3+hexSize], entry[3+hexSize:]
	eout, entry := entry[1:1+hexSize], entry[1+hexSize:]
	esize, entry := entry[1:1+20], entry[1+20:]
	etime, entry := entry[1:1+20], entry[1+20:]
	var buf [HashSize]byte
	if _, err := hex.Decode(buf[:], eid); err != nil {
		return missing(fmt.Errorf("decoding ID: %v", err))
	} else if buf != id {
		return missing(errors.New("mismatched ID"))
	}
	if _, err := hex.Decode(buf[:], eout); err != nil {
		return missing(fmt.Errorf("decoding output ID: %v", err))
	}
	i := 0
	for i < len(esize) && esize[i] == ' ' {
		i++
	}
	size, err := strconv.ParseInt(string(esize[i:]), 10, 64)
	if err != nil {
		return missing(fmt.Errorf("parsing size: %v", err))
	} else if size < 0 {
		return missing(errors.New("negative size"))
	}
	i = 0
	for i < len(etime) && etime[i] == ' ' {
		i++
	}
	tm, err := strconv.ParseInt(string(etime[i:]), 10, 64)
	if err != nil {
		return missing(fmt.Errorf("parsing timestamp: %v", err))
	} else if tm < 0 {
		return missing(errors.New("negative timestamp"))
	}

	c.used(c.fileName(id, "a"))

	return Entry{buf, size, time.Unix(0, tm)}, nil
}

// fileName returns the name of the file corresponding to the given id.
func (c *Cache) fileName(id [HashSize]byte, key string) string {
	return filepath.Join(c.dir, "mycachefile")
}

// Time constants for cache expiration.
//
// We set the mtime on a cache file on each use, but at most one per mtimeInterval (1 hour),
// to avoid causing many unnecessary inode updates. The mtimes therefore
// roughly reflect "time of last use" but may in fact be older by at most an hour.
//
// We scan the cache for entries to delete at most once per trimInterval (1 day).
//
// When we do scan the cache, we delete entries that have not been used for
// at least trimLimit (5 days). Statistics gathered from a month of usage by
// Go developers found that essentially all reuse of cached entries happened
// within 5 days of the previous reuse. See golang.org/issue/22990.
const (
	mtimeInterval = 1 * time.Hour
	trimInterval  = 24 * time.Hour
	trimLimit     = 5 * 24 * time.Hour
)

const (
	// action entry file is "v1 <hex id> <hex out> <decimal size space-padded to 20 bytes> <unixnano space-padded to 20 bytes>\n"
	hexSize   = HashSize * 2
	entrySize = 2 + 1 + hexSize + 1 + hexSize + 1 + 20 + 1 + 20 + 1
)

// used makes a best-effort attempt to update mtime on file,
// so that mtime reflects cache access time.
//
// Because the reflection only needs to be approximate,
// and to reduce the amount of disk activity caused by using
// cache entries, used only updates the mtime if the current
// mtime is more than an hour old. This heuristic eliminates
// nearly all of the mtime updates that would otherwise happen,
// while still keeping the mtimes useful for cache trimming.
func (c *Cache) used(file string) {
	info, err := os.Stat(file)
	if err == nil && c.now().Sub(info.ModTime()) < mtimeInterval {
		return
	}
	os.Chtimes(file, c.now(), c.now())
}

func (c *Cache) put(id ActionID, file io.ReadSeeker, allowVerify bool) (OutputID, int64, error) {
	fmt.Printf("ActionID: %v\n", id)

	// Compute output ID.
	h := sha256.New()
	if _, err := file.Seek(0, 0); err != nil {
		return OutputID{}, 0, err
	}
	size, err := io.Copy(h, file)
	if err != nil {
		return OutputID{}, 0, err
	}
	var out OutputID
	h.Sum(out[:0])

	// Copy to cached output file (if not already present).
	if err := c.copyFile(file, out, size); err != nil {
		return out, size, err
	}

	// Add to cache index.
	return out, size, c.putIndexEntry(id, out, size, allowVerify)
}

// copyFile copies file into the cache, expecting it to have the given
// output ID and size, if that file is not present already.
func (c *Cache) copyFile(file io.ReadSeeker, out OutputID, size int64) error {
	name := c.fileName(out, "d")
	fmt.Printf("copying file to %q\n", name)

	info, err := os.Stat(name)
	if err == nil && info.Size() == size {
		// Check hash.
		if f, err := os.Open(name); err == nil {
			h := sha256.New()
			io.Copy(h, f)
			f.Close()
			var out2 OutputID
			h.Sum(out2[:0])
			if out == out2 {
				return nil
			}
		}
		// Hash did not match. Fall through and rewrite file.
	}

	// Copy file to cache directory.
	mode := os.O_RDWR | os.O_CREATE
	if err == nil && info.Size() > size { // shouldn't happen but fix in case
		mode |= os.O_TRUNC
	}
	f, err := os.OpenFile(name, mode, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	if size == 0 {
		// File now exists with correct size.
		// Only one possible zero-length file, so contents are OK too.
		// Early return here makes sure there's a "last byte" for code below.
		return nil
	}

	// From here on, if any of the I/O writing the file fails,
	// we make a best-effort attempt to truncate the file f
	// before returning, to avoid leaving bad bytes in the file.

	// Copy file to f, but also into h to double-check hash.
	if _, err := file.Seek(0, 0); err != nil {
		f.Truncate(0)
		return err
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	if _, err := io.CopyN(w, file, size-1); err != nil {
		f.Truncate(0)
		return err
	}
	// Check last byte before writing it; writing it will make the size match
	// what other processes expect to find and might cause them to start
	// using the file.
	buf := make([]byte, 1)
	if _, err := file.Read(buf); err != nil {
		f.Truncate(0)
		return err
	}
	h.Write(buf)
	sum := h.Sum(nil)
	if !bytes.Equal(sum, out[:]) {
		f.Truncate(0)
		return fmt.Errorf("file content changed underfoot")
	}

	// Commit cache file entry.
	if _, err := f.Write(buf); err != nil {
		f.Truncate(0)
		return err
	}
	if err := f.Close(); err != nil {
		// Data might not have been written,
		// but file may look like it is the right size.
		// To be extra careful, remove cached file.
		os.Remove(name)
		return err
	}
	os.Chtimes(name, c.now(), c.now()) // mainly for tests
	return nil
}

// putIndexEntry adds an entry to the cache recording that executing the action
// with the given id produces an output with the given output id (hash) and size.
func (c *Cache) putIndexEntry(id ActionID, out OutputID, size int64, allowVerify bool) error {
	// Note: We expect that for one reason or another it may happen
	// that repeating an action produces a different output hash
	// (for example, if the output contains a time stamp or temp dir name).
	// While not ideal, this is also not a correctness problem, so we
	// don't make a big deal about it. In particular, we leave the action
	// cache entries writable specifically so that they can be overwritten.
	//
	// Setting GODEBUG=gocacheverify=1 does make a big deal:
	// in verify mode we are double-checking that the cache entries
	// are entirely reproducible. As just noted, this may be unrealistic
	// in some cases but the check is also useful for shaking out real bugs.
	entry := fmt.Sprintf("v1 %x %x %20d %20d\n", id, out, size, time.Now().UnixNano())
	if verify && allowVerify {
		old, err := c.get(id)
		if err == nil && (old.OutputID != out || old.Size != size) {
			// panic to show stack trace, so we can see what code is generating this cache entry.
			msg := fmt.Sprintf("go: internal cache error: cache verify failed: id=%x changed:<<<\n%s\n>>>\nold: %x %d\nnew: %x %d", id, reverseHash(id), out, size, old.OutputID, old.Size)
			panic(msg)
		}
	}
	file := filepath.Join(c.dir, "cacheindex")

	// Copy file to cache directory.
	mode := os.O_WRONLY | os.O_CREATE
	f, err := os.OpenFile(file, mode, 0666)
	if err != nil {
		return err
	}
	_, err = f.WriteString(entry)
	if err == nil {
		// Truncate the file only *after* writing it.
		// (This should be a no-op, but truncate just in case of previous corruption.)
		//
		// This differs from os.WriteFile, which truncates to 0 *before* writing
		// via os.O_TRUNC. Truncating only after writing ensures that a second write
		// of the same content to the same file is idempotent, and does not — even
		// temporarily! — undo the effect of the first write.
		err = f.Truncate(int64(len(entry)))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		// TODO(bcmills): This Remove potentially races with another go command writing to file.
		// Can we eliminate it?
		os.Remove(file)
		return err
	}
	os.Chtimes(file, c.now(), c.now()) // mainly for tests

	return nil
}

// verify controls whether to run the cache in verify mode.
// In verify mode, the cache always returns errMissing from Get
// but then double-checks in Put that the data being written
// exactly matches any existing entry. This provides an easy
// way to detect program behavior that would have been different
// had the cache entry been returned from Get.
//
// verify is enabled by setting the environment variable
// GODEBUG=gocacheverify=1.
var verify = false

// reverseHash returns the input used to compute the hash id.
func reverseHash(id [HashSize]byte) string {
	hashDebug.Lock()
	s := hashDebug.m[id]
	hashDebug.Unlock()
	return s
}
