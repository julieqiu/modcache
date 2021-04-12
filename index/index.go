package trigram

import "fmt"

type Index struct {
	// hash to filename
	// TODO: what is the hash for
	filehash map[string]string
	// trigram to hash
	trigram map[string][]string
}

// TODO: what information is needed here
type Package struct {
}

func indexDirectory(dir string) {
	// TODO: walk the directory to get the files
	files := []string{}

	// TODO: open and read the file
	for _, filename := range files {
		file := openfile(filename)
		hash := hashfile(file)
		filehash[filename] = hash
		for i := 0; i < len(file)-2; i++ {
			addTrigram(file[i:i+2], filename)
		}
	}
}

func (idx *Index) addTrigram(s string, filename string) {
	// TODO: should i be the document set or the position in the file?
	// Probably just which file it is?
	_, ok := idx.trigram[s]
	if !ok {
		// TODO: how does csearch show the substring
		// Do we need to store the file position here also
		idx.trigram[s] = map[string]bool{
			filename: true,
		}
		return
	}
	idx.trigram[s] = append(idx.trigram[s], filename)
}

// TODO: what should Search output
func (idx *Index) Search(q string) {
	trigrams := queryToTrigrams(q)

	var files []string
	for _, t := range trigrams {
		filenames := idx.trigram[t]
		for f := range filenames {
			// TODO: open file and search for query?
		}
	}
}

func queryToTrigrams(q string) []string {
	// TODO: split query into trigrams
}
