package sshsync

import (
	"bytes"
	"github.com/spf13/afero"
	"log"
	"strings"
)

// protocol constants
const (
	// apply a delta to a file
	// format:
	// delta
	// <number of files>
	// <filename>
	// <delta>
	// server sends no response
	Delta = "delta"

	// stop the server
	Exit = "exit"

	// retrieve a file from server
	// format:
	// get_text_file
	// <filename>
	// server response:
	// <number of lines>
	// text of file
	GetTextFile = "get_text_file"

	// retrieve hash of all files on server
	// response format:
	// <number of files>
	// <md5>
	// <filename>
	GetFileHashes = "get_file_hashes"
)

var endings = []string{
	".cpp",
	".hpp",
	".c",
	".h",
	".go",
	".hs",
	".cl",
	".js",
	".md",
	".txt",
}

// TODO put this in the IgnoreConfig struct
var ignoredPrefixes = []string{
	".git",
	".realtime",
	".idea",
}

type IgnoreConfig struct {
	/*TODO*/
	_unexported int
}

func (c *IgnoreConfig) ShouldIgnore(fs afero.Fs, path string) bool {
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(path, prefix) {
			//log.Println("ignoring ", path)
			return true
		}
	}

	info, err := fs.Stat(path)
	if err == nil && info.IsDir() {
		// skip checking endings on directories
		return false
	} else if err != nil {
		// ignoreConfig things we can't stat
		return true
	}

	for _, ending := range endings {
		if strings.HasSuffix(path, ending) {
			//log.Println("adding ", path)
			return false
		}
	}
	return true
}

func logFatalIfNotNil(label string, err error) {
	if err != nil {
		log.Fatal(label, " error: ", err)
	}
}

func lineCount(text string) int {
	// +1 because newline is separator between lines
	return 1 + bytes.Count([]byte(text), []byte("\n"))
}
