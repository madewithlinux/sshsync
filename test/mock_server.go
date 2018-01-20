package test

import (
	"github.com/Joshua-Wright/sshsync"
	"io"
	"net/rpc"
)

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