package sshsync

import (
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"testing"
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
	server.BuildCache()
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
	server.BuildCache()
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

	// make server commands to execute
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	fmt.Fprintln(stdin, GetFileHashes)
	fmt.Fprintln(stdin, Exit)

	// test
	server := NewServerConfig(serverFs)
	server.BuildCache()
	server.readCommands(stdout, stdin)


	result := stdout.String()
	expected := "1\n" + crc64string(string1) + " testFile.txt\n"
	if result != expected {
		t.Fatalf("%s should have been %s", result, expected)
	}
}

