package sshsync

import (
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/afero"
	"testing"
)

func TestServerWritesDiff(t *testing.T) {
	ServerFs = afero.NewMemMapFs()
	// test data
	string1 := "test string 1"
	string2 := "tested string 222"
	// write test data to file
	afero.WriteFile(ServerFs, "testFile.txt", []byte(string1), 0644)
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
	server := NewServerConfig()
	server.BuildCache()
	server.readCommands(stdout, stdin)

	// verify file now contains string2
	bytes, _ := afero.ReadFile(ServerFs, "testFile.txt")
	string3 := string(bytes)
	if string2 != string3 {
		t.Fatalf("%s should have been %s", string3, string2)
	}
}

func TestServerRetrieveFile(t *testing.T) {
	ServerFs = afero.NewMemMapFs()
	// test data
	string1 := "test string 1\nline two\nline 3"
	// write test data to file
	afero.WriteFile(ServerFs, "testFile.txt", []byte(string1), 0644)

	// make server commands to execute
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	fmt.Fprintln(stdin, GetTextFile)
	fmt.Fprintln(stdin, "testFile.txt")
	fmt.Fprintln(stdin, Exit)

	// test
	server := NewServerConfig()
	server.BuildCache()
	server.readCommands(stdout, stdin)

	result := stdout.String()
	expected := "3\n" + string1 + "\n"
	if result != expected {
		t.Fatalf("%s should have been %s", result, expected)
	}
}
