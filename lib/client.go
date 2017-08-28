package sshsync

import (
	"bytes"
	"fmt"
	// "github.com/d4l3k/go-pry/pry"
	"github.com/fsnotify/fsnotify"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const commitTimeout = 200 * time.Millisecond
const (
	// TODO
	serverBinName = "/home/j0sh/Documents/code/golang-ssh-one-way-sync/cmd/watch_sources_server"
	// serverBinName = "watch_sources_server"
)

type ClientFolder struct {
	BasePath     string
	ClientFs     afero.Fs
	IgnoreCfg    IgnoreConfig
	fileCache    map[string]string
	serverStdout io.Reader
	serverStdin  io.Writer
	exitChannel  chan bool
	// TODO try to not need to put these here directly
	conn    *ssh.Client
	session *ssh.Session
}

func (c *ClientFolder) makePathAbsolute(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.BasePath, path)
}

func (c *ClientFolder) makePathRelative(absPath string) string {
	basePath := c.BasePath
	// make sure ends with slash
	if !strings.HasSuffix(basePath, "/") {
		basePath = basePath + "/"
	}
	// so that we can just trim prefix
	return strings.TrimPrefix(absPath, basePath)
}

func ClientMain() {

	var dir, err = os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	c := &ClientFolder{
		BasePath:  dir,
		fileCache: make(map[string]string),
	}
	c.OpenSshConnection()
	c.StartWatchFiles()

}

// FIXME figure out why this package needs to carry around this object
var dmp = diffmatchpatch.New()

func (c *ClientFolder) sendFileDiffs(files map[string]bool) error {

	buf := &bytes.Buffer{}
	// header
	fmt.Fprintln(buf, Delta)
	fmt.Fprintln(buf, len(files))

	for path := range files {
		log.Println("update: ", path)

		newBuf, err := afero.ReadFile(c.ClientFs, path)
		if err != nil {
			// silently skip files that can't be read
			continue
		}
		newStr := string(newBuf)

		// calculate diff
		diffs := dmp.DiffMain(c.fileCache[path], newStr, false)
		delta := dmp.DiffToDelta(diffs)

		// update cache
		c.fileCache[path] = newStr

		// write to buffer
		fmt.Fprintln(buf, c.makePathRelative(path))
		fmt.Fprintln(buf, delta)
	}
	_, err := fmt.Fprint(c.serverStdin, buf.String())
	if err != nil {
		return err
	}

	return nil
}

func (c *ClientFolder) StopWatchFiles() {
	c.exitChannel <- true
}

func (c *ClientFolder) StartWatchFiles() error {
	// initialize exit channel
	c.exitChannel = make(chan bool)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("failed to get watcher", err)
		return err
	}

	err = c.addWatches(watcher)
	if err != nil {
		log.Println("failed to add watchers", err)
		return err
	}

	go func() {
		var err error
		waitingForCommit := false
		shouldCommit := make(chan bool, 1)
		var filesToAdd = make(map[string]bool)

		for {
			select {
			case <-shouldCommit:
				err := c.sendFileDiffs(filesToAdd)
				if err != nil {
					log.Println("failed to send, will retry")
					waitingForCommit = true
					go func() {
						time.Sleep(commitTimeout)
						shouldCommit <- true
					}()
				} else {
					waitingForCommit = false
					filesToAdd = make(map[string]bool)
				}

			case event := <-watcher.Events:
				absPath := event.Name
				path := c.makePathRelative(absPath)

				if c.IgnoreCfg.ShouldIgnore(c.ClientFs, path) {
					continue
				}

				err = watcher.Add(absPath)
				logFatalIfNotNil("add new watch", err)
				info, err2 := c.ClientFs.Stat(path)

				// do not diff folders
				if err2 == nil && !info.IsDir() {
					filesToAdd[path] = true
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

			case _ = <-c.exitChannel:
				log.Println("quitting watch thread")
				watcher.Close()
				return
			}
		}
	}()

	return nil
}

func (c *ClientFolder) addWatches(watcher *fsnotify.Watcher) error {
	err := watcher.Add(c.BasePath)
	if err != nil {
		log.Println("failed to add base watch", err)
		return err
	}

	return afero.Walk(c.ClientFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !c.IgnoreCfg.ShouldIgnore(c.ClientFs, path) {
			// add watch
			log.Println("path", path)
			log.Println("abs path", c.makePathAbsolute(path))
			err := watcher.Add(c.makePathAbsolute(path))
			logFatalIfNotNil("add initial watch", err)

			if !info.IsDir() {
				// add only files to cache
				buf, err := afero.ReadFile(c.ClientFs, path)
				// TODO do not fail hard
				logFatalIfNotNil("read file", err)
				c.fileCache[path] = string(buf)
			}
		}
		return nil
	})
}

////////////////////////////////////////////

func (c *ClientFolder) OpenLocalConnection(path string) error {
	serverCmd := exec.Command(serverBinName)
	serverCmd.Dir = path

	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := serverCmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = serverCmd.Start()
	if err != nil {
		return err
	}

	c.serverStdout = stdout
	c.serverStdin = stdin

	return nil
}

// TODO parameterize
// TODO return errors
func (c *ClientFolder) OpenSshConnection() {
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
	c.conn, err = ssh.Dial("tcp", "localhost:22" /*FIXME*/ , config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}

	c.session, err = c.conn.NewSession()
	logFatalIfNotNil("start session", err)

	stdin, err := c.session.StdinPipe()
	logFatalIfNotNil("stdin", err)
	stdout, err := c.session.StdoutPipe()
	logFatalIfNotNil("stdout", err)
	fmt.Println("stdin, stdout", stdin, stdout)

	err = c.session.Start( /*FIXME*/
		"/home/j0sh/Documents/code/golang-ssh-one-way-sync/cmd/watch_sources_server " +
			" -path /home/j0sh/Documents/code/golang-ssh-one-way-sync/cmd/")
	logFatalIfNotNil("start server", err)

	c.serverStdout = stdout
	c.serverStdin = stdin
}
