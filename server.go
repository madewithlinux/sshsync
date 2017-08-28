package sshsync

import (
	"bufio"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

type ServerConfig struct {
	ServerFs  afero.Fs
	IgnoreCfg IgnoreConfig
	path      string
	fileCache map[string]string
}

func NewServerConfig(fs afero.Fs) *ServerConfig {
	return &ServerConfig{
		fileCache: make(map[string]string),
		ServerFs:  fs,
	}
}

func (c *ServerConfig) BuildCache() {
	log.Println("recursively caching ", c.path)
	err := afero.Walk(c.ServerFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println("walk err", err)
			return err
		}

		if !c.IgnoreCfg.ShouldIgnore(c.ServerFs, path) {
			log.Println("caching ", path)
			if !info.IsDir() {
				// add only files to cache
				buf, err := afero.ReadFile(c.ServerFs, path)
				logFatalIfNotNil("read file", err)
				c.fileCache[path] = string(buf)
			}
		} else {
			log.Print("ignoring ", path)
		}
		return nil
	})
	logFatalIfNotNil("walk", err)
}

func (c *ServerConfig) readCommands(stdout io.Writer, stdin io.Reader) {
	dmp := diffmatchpatch.New()
	reader := bufio.NewReader(stdin)
	log.Println("start")

	for {
		text, err := reader.ReadString('\n')
		logFatalIfNotNil("read stdin", err)
		// trim newline from end of string
		text = strings.TrimSpace(text)
		log.Println("text: ", text)

		switch text {
		case Delta:
			countStr, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			count, err := strconv.Atoi(strings.TrimSpace(countStr))
			logFatalIfNotNil("read stdin", err)

			for i := 0; i < count; i++ {
				path, err := reader.ReadString('\n')
				logFatalIfNotNil("read path", err)
				path = strings.TrimSpace(path)
				deltaStr, err := reader.ReadString('\n')
				logFatalIfNotNil("read patch", err)
				deltaStr = strings.TrimSpace(deltaStr)

				diffs, err := dmp.DiffFromDelta(c.fileCache[path], deltaStr)
				logFatalIfNotNil("parse delta", err)
				// get updated file content
				newText := dmp.DiffText2(diffs)
				log.Println("new text", newText)
				// write file
				// TODO preserve permissions
				err = afero.WriteFile(c.ServerFs, path, []byte(newText), 0644)
				logFatalIfNotNil("write updated file", err)
				// update cache
				c.fileCache[path] = newText
			}

		case Exit:
			return

		case GetTextFile:
			// assume that ssh connection handles compression,
			// so just send line length then file
			path, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			// trim newline from end of string
			path = strings.TrimSpace(path)
			log.Println("path: ", path)

			fileText := c.fileCache[path]
			fmt.Fprintln(stdout, lineCount(fileText))
			fmt.Fprintln(stdout, fileText)

		case GetFileHashes:
			// respond from cache, do not involve disk
			fmt.Fprintln(stdout, len(c.fileCache))
			for path, text := range c.fileCache {
				fmt.Fprintln(stdout, crc64string(text), path)
			}

		case "get_all_files":
			/*TODO: send tarball?*/
		}
	}

}

func ServerMain() {
	sourceDir := os.Getenv(EnvSourceDir)
	err := os.Chdir(sourceDir)
	logFatalIfNotNil("could not find server source dir", err)

	// log in server-side sources for convenience
	file, err := os.OpenFile("server.log", os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	logFatalIfNotNil("server side open", err)
	defer file.Close()
	log.SetOutput(file)

	server := NewServerConfig(afero.NewOsFs())
	wd, err := os.Getwd()
	logFatalIfNotNil("get cwd", err)

	server.IgnoreCfg = DefaultIgnoreConfig
	server.path = wd
	server.BuildCache()

	server.readCommands(os.Stdout, os.Stdin)
}
