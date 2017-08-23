package sshsync

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

// TODO put in server config struct
var ServerFs afero.Fs = afero.NewOsFs()

type ServerConfig struct {
	ignoreConfig IgnoreConfig
	path         string
	fileCache    map[string]string
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		fileCache: make(map[string]string),
	}
}

func (c *ServerConfig) BuildCache() {
	log.Println("recursively caching ", c.path)
	err := afero.Walk(ServerFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println("walk err", err)
			return err
		}

		if !c.ignoreConfig.ShouldIgnore(ServerFs, path) {
			log.Println("caching ", path)
			if !info.IsDir() {
				// add only files to cache
				buf, err := afero.ReadFile(ServerFs, path)
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
				err = afero.WriteFile(ServerFs, path, []byte(newText), 0644)
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

		case "get_all_files":
			/*TODO: send tarball?*/
		}
	}

}

func ServerMain() {
	// log here for convenience
	/* TODO better logging */
	file, err := os.OpenFile("/home/j0sh/test.txt", os.O_RDWR|os.O_TRUNC, 0644)
	logFatalIfNotNil("server side open", err)
	defer file.Close()
	log.SetOutput(io.MultiWriter(file, os.Stdout))

	wd, err := os.Getwd()
	logFatalIfNotNil("get cwd", err)
	var path = flag.String("path", wd, "directory to serve")
	flag.Parse()
	// cd to path for simplicity
	// TODO use afero.NewBasePathFs
	os.Chdir(*path)

	server := NewServerConfig()
	wd, err = os.Getwd()
	logFatalIfNotNil("get cwd", err)
	server.path = wd
	server.BuildCache()

	server.readCommands(os.Stdout, os.Stdin)

	//reader := bufio.NewReader(os.Stdin)
	//log.Println("start")
	////fmt.Fprintln(file, "stdin fd", os.Stdin.Fd())
	//
	//for {
	//	text, err := reader.ReadString('\n')
	//	logFatalIfNotNil("read stdin", err)
	//	_, err = fmt.Fprint(file, text)
	//	logFatalIfNotNil("write to file", err)
	//	file.Sync()
	//}

}
