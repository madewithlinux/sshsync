package ssh_sync

import (
	"os"
	"log"
	"flag"
	"path/filepath"
	"io/ioutil"
	"io"
	"bufio"
	"strconv"
	"github.com/sergi/go-diff/diffmatchpatch"
)

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
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !c.ignoreConfig.ShouldIgnore(path) {
			log.Println("caching ", path)
			if !info.IsDir() {
				// add only files to cache
				buf, err := ioutil.ReadFile(path)
				logFatalIfNotNil("read file", err)
				c.fileCache[path] = string(buf)
			}
		}
		return nil
	});
}

func (c *ServerConfig) readCommands(stdout io.Writer, stdin io.Reader) {
	dmp := diffmatchpatch.New()
	//scanner := bufio.NewScanner(stdin)
	reader := bufio.NewReader(stdin)
	log.Println("start")
	//fmt.Fprintln(file, "stdin fd", os.Stdin.Fd())

	for {
		text, err := reader.ReadString('\n')
		logFatalIfNotNil("read stdin", err)
		// trim newline from end of string
		text = text[0:len(text)-1]
		//var text string
		//_, err := fmt.Fscanln(stdin, &text)
		//logFatalIfNotNil("read stdin", err)
		//text, err := reader.ReadString('\n')
		//text := scanner.Text()

		switch text {
		case "patch":
			countStr, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			count, err := strconv.Atoi(countStr)
			logFatalIfNotNil("read stdin", err)

			for i := 0; i < count; i++ {
				path, err := reader.ReadString('\n')
				logFatalIfNotNil("read path", err)
				patchStr, err := reader.ReadString('\n')
				logFatalIfNotNil("read patch", err)

				patch, err := dmp.PatchFromText(patchStr)
				logFatalIfNotNil("parse patch", err)
				// update cache
				newText, success := dmp.PatchApply(patch, c.fileCache[path])
				c.fileCache[path] = newText
				for i, s := range success {
					if !s {
						/*TODO: request file from client if didn't work*/
						log.Println("failed patch: ", i, " for ", path)
					}
				}
				/*TODO write new file*/
			}
		case "full_file":
			/*TODO: gzip file, send length, then full file (gzipped binary)*/
		case "get_all_files":
			/*TODO: send tarball?*/
		}
		log.Println("text: ", text)
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
