package sshsync

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/mkideal/cli"
	"github.com/pkg/errors"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"net/rpc"
)

const commitTimeout = 200 * time.Millisecond

// FIXME figure out why this package needs to carry around this object
var dmp = diffmatchpatch.New()

type ClientFolder struct {
	BasePath    string
	ClientFs    afero.Fs
	IgnoreCfg   IgnoreConfig
	FileCache   map[string]string
	ExitChannel chan bool
	Client      *rpc.Client
}

func (c *ClientFolder) Close() {
	c.Client.Close()
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

func (c *ClientFolder) SendFileDiffs(files map[string]bool) error {
	buf := TextFileDeltas{}

	for path := range files {
		log.Println("update: ", path)

		newBuf, err := afero.ReadFile(c.ClientFs, path)
		if err != nil {
			// silently skip files that can't be read
			log.Println("failed to read changed file", err)
			continue
		}
		newStr := string(newBuf)

		// calculate diff
		diffs := dmp.DiffMain(c.FileCache[path], newStr, false)
		delta := dmp.DiffToDelta(diffs)

		// update cache
		c.FileCache[path] = newStr
		// write to buffer
		buf = append(buf, TextFileDelta{c.makePathRelative(path), delta})
	}
	return c.Client.Call(Server_Delta, buf, nil)
}

func (c *ClientFolder) StopWatchFiles() {
	c.ExitChannel <- true
}

func (c *ClientFolder) StartWatchFiles(foreground bool) error {
	// initialize exit channel
	c.ExitChannel = make(chan bool)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("failed to get watcher", err)
		return err
	}

	err = c.AddWatches(watcher)
	if err != nil {
		log.Println("failed to add watchers", err)
		return err
	}

	bgfunc := func() {
		var err error
		waitingForCommit := false
		shouldCommit := make(chan bool, 1)
		var filesToAdd = make(map[string]bool)

		for {
			select {
			case <-shouldCommit:
				err := c.SendFileDiffs(filesToAdd)
				if err != nil {
					log.Println("failed to send, will retry", err)
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
				die("add new watch", err)
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
				die("watcher error", err)

			case _ = <-c.ExitChannel:
				log.Println("quitting watch thread")
				watcher.Close()
				return
			}
		}
	}

	if foreground {
		bgfunc()
	} else {
		go bgfunc()
	}

	return nil
}

func (c *ClientFolder) AddWatches(watcher *fsnotify.Watcher) error {
	err := watcher.Add(c.BasePath)
	if err != nil {
		log.Println("failed to add base watch", err)
		return err
	}

	return afero.Walk(c.ClientFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// explicitly make sure to watch folders (to make sure that new files are watched)
		if info.IsDir() || !c.IgnoreCfg.ShouldIgnore(c.ClientFs, path) {
			log.Println("Path", path)
			log.Println("abs Path", c.makePathAbsolute(path))
			err := watcher.Add(c.makePathAbsolute(path))
			die("add initial watch", err)
		}
		return nil
	})
}

func (c *ClientFolder) BuildCache() error {

	return afero.Walk(c.ClientFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && !c.IgnoreCfg.ShouldIgnore(c.ClientFs, path) {
			// add only files to cache
			buf, err := afero.ReadFile(c.ClientFs, path)
			// TODO do not fail hard
			die("read file", err)
			c.FileCache[path] = string(buf)
		}
		return nil
	})
}

func (c *ClientFolder) getServerChecksums() (map[string]uint64, error) {
	out := make(map[string]uint64)
	err := c.Client.Call(Server_GetFileHashes, 0, &out)
	return out, err
}

func (c *ClientFolder) AssertClientAndServerHashesMatch() error {
	errorText := &bytes.Buffer{}
	fmt.Fprintln(errorText, "Client-Server mismatch:")
	isError := false

	clientChecksums := make(map[string]uint64)
	for path, text := range c.FileCache {
		clientChecksums[path] = crc64checksum(text)
	}

	serverChecksums, err := c.getServerChecksums()
	if err != nil {
		return err
	}

	ignoreChecksumCheck := make(map[string]bool)
	// check for files on Client not on server
	for path, _ := range clientChecksums {
		if _, ok := serverChecksums[path]; !ok {
			fmt.Fprintln(errorText, "on Client, missing from server:", path)
			ignoreChecksumCheck[path] = true
			isError = true
		}
	}
	// check for files on server not on Client
	for path, _ := range serverChecksums {
		if _, ok := clientChecksums[path]; !ok {
			fmt.Fprintln(errorText, "on server, missing from Client:", path)
			ignoreChecksumCheck[path] = true
			isError = true
		}
	}

	// check for Crc64 mismatches, ignoring missing files
	for path, clientChecksum := range clientChecksums {
		if serverChecksums[path] != clientChecksum && !ignoreChecksumCheck[path] {
			fmt.Fprintln(errorText, "Crc64 mismatch:", path)
			isError = true
		}
	}

	if isError {
		return errors.New(errorText.String())
	} else {
		return nil
	}
}

////////////////////////////////////////////

type argT struct {
	cli.Helper
	ServerAddress  string `cli:"*addr" usage:"server address"`
	ServerUsername string `cli:"user" usage:"server username" dft:"$USER"`
	ServerPort     string `cli:"port" usage:"server port" dft:"22"`
	ServerPath     string `cli:"*remote" usage:"server Path"`
	LocalPath      string `cli:"*local" usage:"local Path"`
}

func ClientMain() {

	cli.Run(new(argT), func(ctx *cli.Context) error {
		argv := ctx.Argv().(*argT)

		var dir = argv.LocalPath
		err := os.Chdir(dir)
		die("chdir", err)

		c := &ClientFolder{
			ClientFs: afero.NewBasePathFs(afero.NewOsFs(), dir),
			BasePath: dir,
			// TODO configurable
			IgnoreCfg: DefaultIgnoreConfig,
			FileCache: make(map[string]string),
		}
		defer c.Close()

		conn, err := OpenSshConnection(argv.ServerPath, argv.ServerUsername, argv.ServerAddress+":"+argv.ServerPort)
		die("open ssh connection", err)
		c.Client = rpc.NewClient(conn)
		err = c.BuildCache()
		die("build cache", err)
		for path, _ := range c.FileCache {
			log.Println("cache", path)
		}
		err = c.AssertClientAndServerHashesMatch()
		// TODO
		//err = c.TryAutoResolveWithServerState()
		die("check up to date", err)
		c.StartWatchFiles(true)

		return nil
	})

}
