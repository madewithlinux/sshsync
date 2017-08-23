package sshsync

import (
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	//"github.com/d4l3k/go-pry/pry"
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TODO put in client config struct
var ClientFs afero.Fs = afero.NewOsFs()

const commitTimeout = 200 * time.Millisecond
const (
	serverBinName = "watch_sources_server"
)

type SyncFolder struct {
	ignoreConfig IgnoreConfig
	BaseRepoPath string
	fileCache    map[string]string
	serverStdout io.Reader
	serverStdin  io.Writer
	conn         *ssh.Client
	session      *ssh.Session
}

func ClientMain() {

	var dir, err = os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	r := &SyncFolder{
		BaseRepoPath: dir,
		fileCache:    make(map[string]string),
	}
	r.openSshConnection()
	r.watchFiles()

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
				fmt.Fprintln(buf, Delta)
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

				if r.ignoreConfig.ShouldIgnore(ClientFs, path) {
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

// TODO return err
func (r *SyncFolder) addWatches(watcher *fsnotify.Watcher) {
	err2 := watcher.Add(".")
	logFatalIfNotNil("add watch", err2)

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !r.ignoreConfig.ShouldIgnore(ClientFs, path) {
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
	})
}

////////////////////////////////////////////

func (r *SyncFolder) openLocalConnection(path string) error {
	return nil
	// TODO
}

// TODO parameterize
// TODO return errors
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
		User:            "j0sh", /*FIXME*/
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	// Dial your ssh server.
	r.conn, err = ssh.Dial("tcp", "localhost:22" /*FIXME*/, config)
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

	err = r.session.Start( /*FIXME*/
		"/home/j0sh/Documents/code/golang-ssh-one-way-sync/cmd/watch_sources_server " +
			" -path /home/j0sh/Documents/code/golang-ssh-one-way-sync/cmd/")
	logFatalIfNotNil("start server", err)

	r.serverStdout = stdout
	r.serverStdin = stdin
}
