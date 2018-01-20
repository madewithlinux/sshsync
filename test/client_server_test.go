package test

import (
	"testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/Joshua-Wright/sshsync"
	"net/rpc"
)

func TestClientAssertMatchServer(t *testing.T) {
	testName := "TestClientAssertMatchServer"

	// check that same content works
	WithClientServerFolders(t, testName, func(clientPath string, clientFs afero.Fs, serverFs afero.Fs) {
		var err error
		assert.NoError(t, afero.WriteFile(serverFs, "sameFile.go", []byte("same content"), 0644))
		assert.NoError(t, afero.WriteFile(clientFs, "sameFile.go", []byte("same content"), 0644))
		server := sshsync.NewServerConfig(serverFs)
		server.BuildCache()
		clientConn, serverConn := sshsync.TwoWayPipe()
		go server.ReadCommands(serverConn)
		c := &sshsync.ClientFolder{
			BasePath:  clientPath,
			ClientFs:  clientFs,
			FileCache: make(map[string]string),
			Client:    rpc.NewClient(clientConn),
		}
		c.BuildCache()
		err = c.AssertClientAndServerHashesMatch()
		assert.NoError(t, err)
	})

	// different content in file makes error
	WithClientServerFolders(t, testName, func(clientPath string, clientFs afero.Fs, serverFs afero.Fs) {
		var err error
		assert.NoError(t, afero.WriteFile(serverFs, "differentContent.go", []byte("same content"), 0644))
		assert.NoError(t, afero.WriteFile(clientFs, "differentContent.go", []byte("different content"), 0644))
		server := sshsync.NewServerConfig(serverFs)
		server.BuildCache()
		clientConn, serverConn := sshsync.TwoWayPipe()
		go server.ReadCommands(serverConn)
		c := &sshsync.ClientFolder{
			BasePath:  clientPath,
			ClientFs:  clientFs,
			FileCache: make(map[string]string),
			Client:    rpc.NewClient(clientConn),
		}
		c.BuildCache()
		err = c.AssertClientAndServerHashesMatch()
		assert.Error(t, err)
	})

	// completely different files makes error
	WithClientServerFolders(t, testName, func(clientPath string, clientFs afero.Fs, serverFs afero.Fs) {
		var err error
		assert.NoError(t, afero.WriteFile(serverFs, "serverFile.go", []byte("same content"), 0644))
		assert.NoError(t, afero.WriteFile(clientFs, "clientFile.go", []byte("different content"), 0644))
		server := sshsync.NewServerConfig(serverFs)
		server.BuildCache()
		clientConn, serverConn := sshsync.TwoWayPipe()
		go server.ReadCommands(serverConn)
		c := &sshsync.ClientFolder{
			BasePath:  clientPath,
			ClientFs:  clientFs,
			FileCache: make(map[string]string),
			Client:    rpc.NewClient(clientConn),
		}
		c.BuildCache()
		err = c.AssertClientAndServerHashesMatch()
		assert.Error(t, err)
	})
}

// TODO test Client/server startup negotiation code
