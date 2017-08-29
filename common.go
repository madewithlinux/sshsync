package sshsync

import (
	"bytes"
	"github.com/spf13/afero"
	"github.com/gobwas/glob"
	"log"
	"strings"
	"hash/crc64"
	"fmt"
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

	// send a text file to the server
	// format:
	// <number of lines>
	// <raw text file>
	SendTextFile = "send_text_file"

	// retrieve hash of all files on server
	// response format:
	// <number of files>
	// <crc64>
	// <filename>
	GetFileHashes = "get_file_hashes"

	// environment constants
	// used by the client to pass parameters to the server
	// (to avoid shell interpolation)
	// must start with LC_ to go through ssh for some reason
	EnvSourceDir = "LC_SSHSYNC_SOURCE_DIR"
	EnvIgnoreCfg = "LC_SSHSYNC_IGNORE_CFG"

	BinName = "sshsync"
)

// TODO serialize this so it can go in env
type IgnoreConfig struct {
	Extensions []string
	// glob matched
	GlobIgnore         []string
	compiledGlobIgnore []glob.Glob
}

var DefaultIgnoreConfig = IgnoreConfig{
	Extensions: []string{
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
	},
	GlobIgnore: []string{
		// ignore all hidden files and folders
		".*",
		// ignore build folders
		"build/*",
		"target/*",
		"out/*",
	},
}

// call this before using compiled glob patterns
func (cfg *IgnoreConfig) compileGlobs() {
	if len(cfg.GlobIgnore) == len(cfg.compiledGlobIgnore) {
		return
	}
	cfg.compiledGlobIgnore = make([]glob.Glob, len(cfg.GlobIgnore))
	for i, globIgnoreString := range cfg.GlobIgnore {
		var err error
		cfg.compiledGlobIgnore[i], err = glob.Compile(globIgnoreString)
		if err != nil {
			log.Println("bad glob pattern:", globIgnoreString, err)
			panic("bad glob pattern: " + globIgnoreString + " " + err.Error())
		}
	}
}

func (cfg *IgnoreConfig) ShouldIgnore(fs afero.Fs, path string) bool {
	// if zero-initialized, ignore only what can't be stat
	if len(cfg.Extensions) == 0 &&
		len(cfg.GlobIgnore) == 0 &&
		len(cfg.compiledGlobIgnore) == 0 {
		log.Println("default not ignoring", path)
		stat, err := fs.Stat(path)
		if err != nil && stat.IsDir() {
			log.Println("default ignoring dir", path)
			return true
		} else if err != nil && !stat.IsDir() {
			log.Println("default not ignoring", path)
			return false
		} else {
			log.Println("default ignoring non existent", path)
			return false
		}
	}

	cfg.compileGlobs()
	for _, globIgnore := range cfg.compiledGlobIgnore {
		if globIgnore.Match(path) {
			log.Println("ignoring", path)
			return true
		}
	}

	info, err := fs.Stat(path)
	if err == nil && info.IsDir() {
		// skip checking endings on directories
		log.Println("ignoring dir", path)
		return true
	} else if err != nil {
		// ignore things we can't stat
		log.Println("ignoring non existent", path)
		return true
	}

	// do not ignore whitelisted extensions
	for _, extension := range cfg.Extensions {
		if strings.HasSuffix(path, extension) {
			log.Println("not ignoring", path)
			return false
		}
	}
	// ignore everything else
	log.Println("default ignoring", path)
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

var ecmaTable = crc64.MakeTable(crc64.ECMA)

func crc64checksum(content string) uint64 {
	return crc64.Checksum([]byte(content), ecmaTable)
}

func crc64string(content string) string {
	return fmt.Sprintf("%0X", crc64checksum(content))
}
