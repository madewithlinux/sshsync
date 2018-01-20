package sshsync

import (
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"testing"
	"io"
	"net/rpc"
)

func TestServer(t *testing.T) {
	var serverFs = afero.NewMemMapFs()
	// setup
	string1 := "test string 1\nline two"
	string2 := "tested string 222\nline 2"
	// write test data to file
	afero.WriteFile(serverFs, "testFile.txt", []byte(string1), 0644)
	// get delta
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string1, string2, false)
	delta := dmp.DiffToDelta(diffs)

	// make server commands to execute
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	fmt.Fprintln(stdin, Delta)
	fmt.Fprintln(stdin, "1")
	fmt.Fprintln(stdin, "testFile.txt")
	fmt.Fprintln(stdin, delta)
	fmt.Fprintln(stdin, Exit)

	// test
	server := NewServerConfig(serverFs)
	server.buildCache()
	server.readCommands(stdout, stdin)

	// verify file now contains string2
	fileBytes, _ := afero.ReadFile(serverFs, "testFile.txt")
	string3 := string(fileBytes)
	if string2 != string3 {
		t.Fatalf("%s should have been %s", string3, string2)
	}

	// test get text file
	// read contents of testFile.txt
	fmt.Fprintln(stdin, GetTextFile)
	fmt.Fprintln(stdin, "fileToRead.txt")
	fmt.Fprintln(stdin, Exit)
	stdout.Reset()
	afero.WriteFile(serverFs, "fileToRead.txt", []byte("line 1\nline two\nthird line\n"), 0644)

	// test
	server.buildCache()
	server.readCommands(stdout, stdin)

	result := stdout.String()
	expected := "27\nline 1\nline two\nthird line\n\n"
	if result != expected {
		t.Fatalf("%s should have been %s", result, expected)
	}
}

func TestServerGetHashes(t *testing.T) {
	// test just one file because ordering is difficult to compare
	var serverFs = afero.NewMemMapFs()
	// setup
	string1 := "test string 1\nline two"
	// write test data to file
	afero.WriteFile(serverFs, "testFile.txt", []byte(string1), 0644)

	// server read, client write (and vice versa)
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	// test
	server := NewServerConfig(serverFs)
	server.buildCache()
	go server.readCommands(sw, sr)
	conn := &ReadWriteCloseAdapter{cr, cw}
	client := rpc.NewClient(conn)

	var out ChecksumIndex
	err := client.Call(ServerConfig_GetFileHashes, 0, &out)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if _, ok := out["testFile.txt"]; !ok {
		t.Fatalf("file not found in index. index: %s", out)
	}
	sr.Close()
	cr.Close()
}
