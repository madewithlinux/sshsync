package main

import (
	"github.com/fsnotify/fsnotify"
	//"github.com/d4l3k/go-pry/pry"
	"github.com/sergi/go-diff/diffmatchpatch"
	"log"
	"path/filepath"
	"os"
	"strings"
	"fmt"
	"time"
	"io/ioutil"
	"bytes"
	"net"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh"
	"io"
	"bufio"
)

var endings []string = []string{
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

var ignored_prefixes = []string{
	".git",
	".realtime",
	".idea",
}

const commitTimeout = 200 * time.Millisecond

func shouldIgnore(path string) bool {
	for _, prefix := range ignored_prefixes {
		if strings.HasPrefix(path, prefix) {
			//log.Println("ignoring ", path)
			return true
		}
	}

	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		// skip checking endings on directories
		return false
	} else if err != nil {
		// ignore things we can't stat
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

type SyncFolder struct {
	BaseRepoPath string
	fileCache    map[string]string
	serverStdout io.Reader
	serverStdin  io.Writer
	conn         *ssh.Client
	session      *ssh.Session
}

func main() {


	if len(os.Args) >= 2 && os.Args[1] == "-server" {
		//if len(os.Args) > 1 {
		//if false {

		file, err := os.OpenFile("/home/j0sh/test.txt", os.O_RDWR|os.O_TRUNC, 0644)
		/*FIXME*/
		logFatalIfNotNil("server side open", err)
		defer file.Close()
		// log here for convenience
		log.SetOutput(file)

		reader := bufio.NewReader(os.Stdin)
		log.Println("start")
		//fmt.Fprintln(file, "stdin fd", os.Stdin.Fd())

		for {
			text, err := reader.ReadString('\n')
			logFatalIfNotNil("read stdin", err)
			_, err = fmt.Fprint(file, text)
			logFatalIfNotNil("write to file", err)
			file.Sync()
		}

	} else {
		//pry.Pry()
		var dir, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		//fmt.Println(dir)
		//io.Copy(os.Stdout, stdout)

		r := &SyncFolder{
			BaseRepoPath: dir,
			fileCache:    make(map[string]string),
		}
		r.openSshConnection()
		r.watchFiles()
	}
}

func (r *SyncFolder) watchFiles() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	r.addWatches(watcher)

	done := make(chan bool)
	go func() {
		dmp := diffmatchpatch.New()
		var err error
		waitingForCommit := false
		shouldCommit := make(chan bool, 1)
		var filesToAdd = make(map[string]bool)

		for {
			select {
			case <-shouldCommit:
				waitingForCommit = false

				buf := &bytes.Buffer{}
				// header
				fmt.Fprintln(buf, "diff")
				fmt.Fprintln(buf, len(filesToAdd))

				for path, _ := range filesToAdd {
					log.Println("update: ", path)

					// TODO make sure file still exists (skip otherwise?)
					newBuf, err := ioutil.ReadFile(path)
					logFatalIfNotNil("read new file", err)
					newStr := string(newBuf)

					// calculate diff
					diffs := dmp.DiffMain(r.fileCache[path], newStr, false)
					delta := dmp.DiffToDelta(diffs)

					// update cache
					r.fileCache[path] = newStr

					// write to buffer
					fmt.Fprintln(buf, path)
					fmt.Fprintln(buf, delta)
				}
				// TODO: write to server async?
				_, err := fmt.Fprint(r.serverStdin, buf.String())
				logFatalIfNotNil("write to server", err)

			case event := <-watcher.Events:
				path := event.Name

				if shouldIgnore(path) {
					continue
				}

				err = watcher.Add(path)
				logFatalIfNotNil("add new watch", err)
				info, err2 := os.Stat(path)

				// do not diff folders
				if err2 == nil && !info.IsDir() {
					// for some reason paths are not normalized
					filesToAdd[strings.TrimPrefix(path, "./")] = true
				}

				if !waitingForCommit {
					waitingForCommit = true
					go func() {
						time.Sleep(commitTimeout)
						shouldCommit <- true
					}()
				}

			case err := <-watcher.Errors:
				logFatalIfNotNil("watcher error", err)
			}
		}
	}()

	/*FIXME don't just infinite wait?*/
	<-done
}

func (r *SyncFolder) addWatches(watcher *fsnotify.Watcher) {
	err2 := watcher.Add(".")
	logFatalIfNotNil("add watch", err2)

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !shouldIgnore(path) {
			// add watch
			log.Println("path", path)
			err := watcher.Add(path)
			logFatalIfNotNil("add initial watch", err)

			if !info.IsDir() {
				// add only files to cache
				buf, err := ioutil.ReadFile(path)
				logFatalIfNotNil("read file", err)
				r.fileCache[path] = string(buf)
			}
		}
		return nil
	});
}

////////////////////////////////////////////

func (r *SyncFolder) openSshConnection() {
	// FIXME hard coded stuff
	// FIXME error handling

	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	authAgent := agent.NewClient(sock)
	signers, err := authAgent.Signers()
	logFatalIfNotNil("get signers", err)
	auths := []ssh.AuthMethod{ssh.PublicKeys(signers...)}

	config := &ssh.ClientConfig{
		User:            "j0sh"/*FIXME*/,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	// Dial your ssh server.
	r.conn, err = ssh.Dial("tcp", "localhost:22"/*FIXME*/, config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}

	r.session, err = r.conn.NewSession()
	logFatalIfNotNil("start session", err)

	stdin, err := r.session.StdinPipe()
	logFatalIfNotNil("stdin", err)
	stdout, err := r.session.StdoutPipe()
	logFatalIfNotNil("stdout", err)
	fmt.Println("stdin, stdout", stdin, stdout)

	err = r.session.Start("/home/j0sh/Documents/code/golang-ssh-one-way-sync/watch_sources -server"/*FIXME*/)
	logFatalIfNotNil("start server", err)

	r.serverStdout = stdout
	r.serverStdin = stdin
}
