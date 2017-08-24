package sshsync

import (
	"bytes"
	"fmt"
	"log"
	// "github.com/d4l3k/go-pry/pry"
	"github.com/spf13/afero"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	// err = os.Chdir(testName)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer os.Chdir("..")

	var baseFs afero.Fs = afero.NewOsFs()
	clientFs := afero.NewBasePathFs(baseFs, testName)
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
	// sleep to make sure that testfile1 is diff'ed before newfile
	time.Sleep(100 * time.Millisecond)
	// create new file
	err = afero.WriteFile(clientFs, "newfile.txt", []byte("new\n\tcontent\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// sleep to let client see progress
	time.Sleep(500 * time.Millisecond)

	result := serverStdin.String()
	expected := Delta + "\n" +
		"2\n" +
		"testfile1.txt\n" +
		"=5\t-1\t+2%0A\n" +
		"newfile.txt\n" +
		"+new%0A%09content%0A\n"
	if result != expected {
		t.Log("len(result):", len(result))
		t.Log("len(expected):", len(expected))
		t.Fatalf("%s should have been %s", result, expected)
	}
}
