package sshsync

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"log"
	// "github.com/d4l3k/go-pry/pry"
	"github.com/spf13/afero"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClientSendFileDiffs(t *testing.T) {
	testName := "TestClientSendFileDiffs"

	err := os.Mkdir(testName, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testName)
	clientPath, err := filepath.Abs(testName)
	if err != nil {
		t.Fatal(err)
	}

	clientFs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	err = afero.WriteFile(clientFs, "testfile1.txt", []byte("test 1"), 0644)
	if err != nil {
		t.Fatal(err)
	}

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

	// add watches just to build the cache
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	err = c.addWatches(watcher)
	if err != nil {
		t.Fatal(err)
	}

	// update existing file
	file, err := clientFs.OpenFile("testfile1.txt", os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintln(file, "test 2")
	if err != nil {
		t.Fatal(err)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// create new file
	err = afero.WriteFile(clientFs, "newfile.txt", []byte("new\n\tcontent\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = c.sendFileDiffs(map[string]bool{
		"testfile1.txt": true,
		"newfile.txt":   true,
	})
	if err != nil {
		t.Fatal("should not have failed", err)
	}

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
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testName)
	clientPath, err := filepath.Abs(testName)
	if err != nil {
		t.Fatal(err)
	}

	clientFs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	err = afero.WriteFile(clientFs, "testfile1.txt", []byte("test 1"), 0644)
	if err != nil {
		t.Fatal(err)
	}

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

	go func() {
		log.Println("test1")
		err := c.watchFiles()
		if err != nil {
			t.Fatal(err)
		}
	}()
	// sleep to let client setup watches
	time.Sleep(500 * time.Millisecond)

	// update existing file
	file, err := clientFs.OpenFile("testfile1.txt", os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintln(file, "test 2")
	if err != nil {
		t.Fatal(err)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// create new file
	err = afero.WriteFile(clientFs, "newfile.txt", []byte("new\n\tcontent\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

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