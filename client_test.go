package sshsync

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/Joshua-Wright/sshsync"
	"net/rpc"
	"reflect"
	"path/filepath"
	"io"
)

func WithFolder(t *testing.T, testName string, f func(absPath string, fs afero.Fs)) {
	err := os.Mkdir(testName, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testName)
	absPath, err := filepath.Abs(testName)
	assert.NoError(t, err)
	fs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	f(absPath, fs)
}


func TestClientSendFileDiffs(t *testing.T) {
	testName := "TestClientSendFileDiffs"
	WithFolder(t, testName, func(clientPath string, clientFs afero.Fs) {
		var err error
		err = afero.WriteFile(clientFs, "testfile1.txt", []byte("test 1"), 0644)
		assert.NoError(t, err)
		t.Log(clientPath)

		clientConn, serverConn := sshsync.TwoWayPipe()
		defer clientConn.Close()
		defer serverConn.Close()
		server := MockServer{}
		go server.ReadCommands(serverConn)

		c := &sshsync.ClientFolder{
			BasePath:  clientPath,
			ClientFs:  clientFs,
			FileCache: make(map[string]string),
			Client:    rpc.NewClient(clientConn),
		}

		err = c.BuildCache()
		assert.NoError(t, err)

		// add watches just to build the cache
		watcher, err := fsnotify.NewWatcher()
		assert.NoError(t, err)
		defer watcher.Close()
		err = c.AddWatches(watcher)
		assert.NoError(t, err)

		// update existing file
		file, err := clientFs.OpenFile("testfile1.txt", os.O_RDWR, 0644)
		assert.NoError(t, err)
		_, err = fmt.Fprintln(file, "test 2")
		assert.NoError(t, err)
		err = file.Close()
		assert.NoError(t, err)
		// create new file
		err = afero.WriteFile(clientFs, "newfile.txt", []byte("new\n\tcontent\n"), 0644)
		assert.NoError(t, err)

		err = c.SendFileDiffs(map[string]bool{
			"testfile1.txt": true,
			"newfile.txt":   true,
		})
		assert.NoError(t, err)

		result := server.CallsDelta[0]
		expected2 := sshsync.TextFileDeltas{
			{"newfile.txt", "+new%0A%09content%0A"},
			{"testfile1.txt", "=5\t-1\t+2%0A"},
		}
		expected1 := sshsync.TextFileDeltas{
			{"testfile1.txt", "=5\t-1\t+2%0A"},
			{"newfile.txt", "+new%0A%09content%0A"},
		}
		if !reflect.DeepEqual(result, expected1) && !reflect.DeepEqual(result, expected2) {
			t.Log("len(result):", len(result))
			t.Log("len(expected):", len(expected1))
			t.Fatalf("%s should have been %s", result, expected1)
		}
	})
}

func AssertFileContent(t *testing.T, fs afero.Fs, path string, content string) {
	fileBytes, err := afero.ReadFile(fs, path)
	assert.NoError(t, err)
	assert.Equal(t, content, string(fileBytes))
}

type MockServer struct {
	CallsDelta          []sshsync.TextFileDeltas
	FileHashes          sshsync.ChecksumIndex
	CallsGetTextFile    []string
	ResponseGetTextFile map[string]string
	CallsSendTextFile   []sshsync.TextFile
	server    *rpc.Server
}

func (c *MockServer) Delta(deltas sshsync.TextFileDeltas, _ *int) error {
	c.CallsDelta = append(c.CallsDelta, deltas)
	return nil
}

func (c *MockServer) GetFileHashes(_ int, index *sshsync.ChecksumIndex) error {
	*index = c.FileHashes
	return nil
}

func (c *MockServer) GetTextFile(path string, content *string) error {
	c.CallsGetTextFile = append(c.CallsGetTextFile, path)
	*content = c.ResponseGetTextFile[path]
	return nil
}

func (c *MockServer) SendTextFile(file sshsync.TextFile, _ *int) error {
	c.CallsSendTextFile = append(c.CallsSendTextFile, file)
	return nil
}

func (c *MockServer) ReadCommands(conn io.ReadWriteCloser) {
	c.server = rpc.NewServer()
	c.server.RegisterName("Server", c)
	c.server.ServeConn(conn)
}