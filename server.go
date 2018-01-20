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
	"net/rpc"
)

const(
	ServerConfig_GetFileHashes = "ServerConfig.GetFileHashes"
)

type ServerConfig struct {
	ServerFs  afero.Fs
	IgnoreCfg IgnoreConfig
	path      string
	fileCache map[string]string
	server    *rpc.Server
}

func NewServerConfig(fs afero.Fs) *ServerConfig {
	return &ServerConfig{
		fileCache: make(map[string]string),
		// TODO configurable
		IgnoreCfg: DefaultIgnoreConfig,
		ServerFs:  fs,
	}
}

func (c *ServerConfig) buildCache() {
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

func (c *ServerConfig) readCommands(stdout io.WriteCloser, stdin io.Reader) {
	c.server = rpc.NewServer()
	c.server.Register(c)
	c.server.ServeConn(&ReadWriteCloseAdapter{stdin, stdout})
	return

	dmp := diffmatchpatch.New()
	reader := bufio.NewReader(stdin)
	log.Println("start")

	for {
		text, err := reader.ReadString('\n')
		logFatalIfNotNil("read stdin", err)
		// trim newline from end of string
		text = strings.TrimSpace(text)
		log.Println("text:", text)

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
			log.Println("sending: ", path)

			fileText := c.fileCache[path]
			fmt.Fprintln(stdout, len([]byte(fileText)))
			fmt.Fprintln(stdout, fileText)

		case SendTextFile:
			log.Println("receiving something")

			path, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			// remove newline
			path = path[0:len(path)-1]
			log.Println("receiving:", path)

			countStr, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			log.Println("size:", countStr)

			byteCount, err := strconv.Atoi(strings.TrimSpace(countStr))
			logFatalIfNotNil("convert byte count", err)

			fileBytes := make([]byte, byteCount)
			_, err = io.ReadFull(reader, fileBytes)
			logFatalIfNotNil("read file bytes", err)
			// read trailing newline
			reader.ReadByte()

			fileText := string(fileBytes)

			// write file to cache
			c.fileCache[path] = fileText
			// TODO file mode?
			err = afero.WriteFile(c.ServerFs, path, fileBytes, 0644)
			logFatalIfNotNil("write file", err)

		//case GetFileHashes:
		//	// respond from cache, do not involve disk
		//	fmt.Fprintln(stdout, len(c.fileCache))
		//	for path, text := range c.fileCache {
		//		fmt.Fprintln(stdout, crc64string(text), path)
		//	}

		case "get_all_files":
			log.Fatal("not implemented")
			/*TODO: send tarball?*/

		default:
			log.Fatal("bad input:", text)
		}
	}

}

func (c *ServerConfig) GetFileHashes(i int, index *ChecksumIndex) error {
	m := make(ChecksumIndex)
	for path, text := range c.fileCache {
		log.Println(path)
		m[path] = crc64checksum(text)
	}
	*index = m
	return nil
}

func (c *ServerConfig) GetTextFile(path string, content *string) error {
	*content = c.fileCache[path]
	return nil
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
	server.buildCache()

	server.readCommands(os.Stdout, os.Stdin)
}
