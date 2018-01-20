package sshsync

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	"os"
	"path/filepath"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
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

	c := &ClientFolder{
		BasePath:  clientPath,
		ClientFs:  clientFs,
		fileCache: make(map[string]string),
	}
	serverStdin := &bytes.Buffer{}
	serverStdout := &bytes.Buffer{}
	c.serverStdin = serverStdin
	c.serverStdout = serverStdout

	c.BuildCache()
	assert.NoError(t, err)

	// add watches just to build the cache
	watcher, err := fsnotify.NewWatcher()
	assert.NoError(t, err)
	defer watcher.Close()
	err = c.addWatches(watcher)
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

	err = c.sendFileDiffs(map[string]bool{
		"testfile1.txt": true,
		"newfile.txt":   true,
	})
	assert.NoError(t, err)

	result := serverStdin.String()
	expected2 := Delta + "\n" +
		"2\n" +
		"newfile.txt\n" +
		"+new%0A%09content%0A\n" +
		"testfile1.txt\n" +
		"=5\t-1\t+2%0A\n"
	expected1 := Delta + "\n" +
		"2\n" +
		"testfile1.txt\n" +
		"=5\t-1\t+2%0A\n" +
		"newfile.txt\n" +
		"+new%0A%09content%0A\n"
	if result != expected1 && result != expected2 {
		t.Log("len(result):", len(result))
		t.Log("len(expected):", len(expected1))
		t.Fatalf("%s should have been %s", result, expected1)
	}
}

func TestClientWritesDiff(t *testing.T) {
	testName := "TestClientWritesDiff"

	err := os.Mkdir(testName, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testName)
	clientPath, err := filepath.Abs(testName)
	assert.NoError(t, err)

	clientFs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	err = afero.WriteFile(clientFs, "testfile1.txt", []byte("test 1"), 0644)
	assert.NoError(t, err)

	t.Log(clientPath)

	c := &ClientFolder{
		IgnoreCfg: DefaultIgnoreConfig,
		BasePath:  clientPath,
		ClientFs:  clientFs,
		fileCache: make(map[string]string),
	}
	serverStdin := &bytes.Buffer{}
	serverStdout := &bytes.Buffer{}
	c.serverStdin = serverStdin
	c.serverStdout = serverStdout

	c.BuildCache()
	assert.NoError(t, err)
	err = c.StartWatchFiles(false)
	assert.NoError(t, err)

	// sleep to let client setup watches
	time.Sleep(500 * time.Millisecond)

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

	// sleep to let client see progress
	time.Sleep(500 * time.Millisecond)

	result := serverStdin.String()
	expected2 := Delta + "\n" +
		"2\n" +
		"newfile.txt\n" +
		"+new%0A%09content%0A\n" +
		"testfile1.txt\n" +
		"=5\t-1\t+2%0A\n"
	expected1 := Delta + "\n" +
		"2\n" +
		"testfile1.txt\n" +
		"=5\t-1\t+2%0A\n" +
		"newfile.txt\n" +
		"+new%0A%09content%0A\n"
	if result != expected1 && result != expected2 {
		t.Log("len(result):", len(result))
		t.Log("len(expected):", len(expected1))
		t.Fatalf("%s should have been %s", result, expected1)
	}
}

// TODO test client/server startup negotiation code