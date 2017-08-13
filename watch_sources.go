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
	"compress/gzip"
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

func zlibCompress(in string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	fmt.Fprint(w, in)
	w.Close()
	return b.Bytes()
}

func zlibCompressBytes(in []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(in)
	w.Close()
	return b.Bytes()
}

func logFatalIfNotNil(label string, err error) {
	if err != nil {
		log.Fatal(label, err)
	}
}

type SyncFolder struct {
	BaseRepoPath string
	fileCache    map[string]string
	serverStdout io.Reader
	serverStdin  io.WriteCloser
}

type Delta struct {
	Filename string
	Data     string
}
type DeltaSet struct {
	Deltas []Delta
}

func main() {

	//fmt.Println(os.Args)

	//if len(os.Args) >= 2 && os.Args[1] == "-server" {
	if len(os.Args) > 1 {
		fmt.Println("I am the server")
		file, err := os.Open("~/test.txt")
		logFatalIfNotNil("server side open", err)
		reader := bufio.NewReader(os.Stdin)
		for {
			text, _ := reader.ReadString('\n')
			fmt.Fprintln(file, text)
			file.Sync()
		}
	} else {
		//pry.Pry()
		var dir, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		//fmt.Println(dir)


		stdout, stdin := openSshConnection()
		//io.Copy(os.Stdout, stdout)

		r := &SyncFolder{
			BaseRepoPath: dir,
			fileCache:    make(map[string]string),
			serverStdout: stdout,
			serverStdin:  stdin,
		}
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

	//receive, send := libchan.Pipe()
	//
	//go func() {
	//	for {
	//		ds := &DeltaSet{}
	//		err := receive.Receive(ds)
	//		logFatalIfNotNil("receive", err)
	//		fmt.Println("ds:", *ds)
	//	}
	//}()

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
				fmt.Fprintln(buf, "diff")
				fmt.Fprintln(buf, len(filesToAdd))

				deltas := &DeltaSet{[]Delta{}}
				for path, _ := range filesToAdd {
					log.Println(path)
					// TODO make sure file still exists
					newBuf, err := ioutil.ReadFile(path)
					logFatalIfNotNil("read new file", err)
					newStr := string(newBuf)

					//oldStr := r.fileCache[path]
					//fmt.Println(
					//	"newlen", len(newStr),
					//	"oldlen", len(oldStr),
					//)

					diffs := dmp.DiffMain(r.fileCache[path], newStr, false)
					delta := dmp.DiffToDelta(diffs)
					//fmt.Println("diffs:", delta)
					//fmt.Println("len:", len(delta))
					//fmt.Println("zlib len:", len(zlibCompress(delta)))
					// update cache
					r.fileCache[path] = newStr

					// write to buffer
					fmt.Fprintln(buf, path)
					fmt.Fprintln(buf, delta)

					deltaStruct := Delta{path, delta}
					deltas.Deltas = append(deltas.Deltas, deltaStruct)
				}
				//fmt.Println("buffer:\n" + buf.String())
				fmt.Fprint(r.serverStdin, buf.String())
				//logFatalIfNotNil("send", send.Send(deltas))
				//var buf bytes.Buffer
				//struc.Pack(&buf, deltas)
				//fmt.Println("delta len:", len(buf.Bytes()))
				//fmt.Println("delta zlib len:", len(zlibCompressBytes(buf.Bytes())))
				//o := &DeltaSet{}
				//err = struc.Unpack(&buf, o)
				//fmt.Println("decode error:", err)

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

func openSshConnection() (io.Reader, io.WriteCloser) {
	// FIXME hard coded stuff
	// FIXME error handling

	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	agent := agent.NewClient(sock)
	signers, err := agent.Signers()
	if err != nil {
		log.Fatal(err)
	}
	auths := []ssh.AuthMethod{ssh.PublicKeys(signers...)}

	config := &ssh.ClientConfig{
		User:            "j0sh",
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	// Dial your ssh server.
	conn, err := ssh.Dial("tcp", "localhost:22", config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}
	defer conn.Close()

	l, err := conn.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatal("unable to register tcp forward: ", err)
	}
	defer l.Close()

	session, err := conn.NewSession()
	stdin, err := session.StdinPipe()
	logFatalIfNotNil("stdin", err)
	stdout, err := session.StdoutPipe()
	logFatalIfNotNil("stdout", err)

	//err = session.Start("/home/j0sh/Documents/code/golang-ssh-one-way-sync/watch_sources -server")
	err = session.Run("/home/j0sh/Documents/code/golang-ssh-one-way-sync/watch_sources -server")
	logFatalIfNotNil("start server", err)

	return stdout, stdin
}
