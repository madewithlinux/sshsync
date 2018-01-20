package test

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	"os"
	"path/filepath"
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/Joshua-Wright/sshsync"
	"net/rpc"
	"reflect"
)

func TestClientSendFileDiffs(t *testing.T) {
	testName := "TestClientSendFileDiffs"

	err := os.Mkdir(testName, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testName)
	clientPath, err := filepath.Abs(testName)
	assert.NoError(t, err)

	clientFs := afero.NewBasePathFs(afero.NewOsFs(), testName)
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
}

// TODO test Client/server startup negotiation code
