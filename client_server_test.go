package sshsync_test

import (
	"testing"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/Joshua-Wright/sshsync"
	"net/rpc"
	"os"
	"path/filepath"
)
func WithClientServerFolders(t *testing.T, testName string, f func(absPath string, clientFs afero.Fs, serverFs afero.Fs)) {
	err := os.Mkdir(testName, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testName)
	absPath, err := filepath.Abs(testName)
	assert.NoError(t, err)
	clientFs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	var serverFs = afero.NewMemMapFs()
	f(absPath, clientFs, serverFs)
}


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
