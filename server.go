package sshsync

import (
	"github.com/spf13/afero"
	"io"
	"log"
	"os"
	"net/rpc"
)

const (
	Server_GetFileHashes = "Server.GetFileHashes"
	Server_GetTextFile   = "Server.GetTextFile"
	Server_SendTextFile  = "Server.SendTextFile"
	Server_Delta         = "Server.Delta"
)

type ServerConfig struct {
	ServerFs  afero.Fs
	IgnoreCfg IgnoreConfig
	path      string
	fileCache map[string]string
	server    *rpc.Server
}

func NewServerConfig(fs afero.Fs) *ServerConfig {
	return &ServerConfig{
		fileCache: make(map[string]string),
		// TODO configurable
		IgnoreCfg: DefaultIgnoreConfig,
		ServerFs:  fs,
	}
}

func (c *ServerConfig) BuildCache() {
	log.Println("recursively caching ", c.path)
	err := afero.Walk(c.ServerFs, ".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println("walk err", err)
			return err
		}

		if !c.IgnoreCfg.ShouldIgnore(c.ServerFs, path) {
			log.Println("caching ", path)
			if !info.IsDir() {
				// add only files to cache
				buf, err := afero.ReadFile(c.ServerFs, path)
				die("read file", err)
				c.fileCache[path] = string(buf)
			}
		} else {
			log.Print("ignoring ", path)
		}
		return nil
	})
	die("walk", err)
}

func (c *ServerConfig) ReadCommands(conn io.ReadWriteCloser) {
	c.server = rpc.NewServer()
	c.server.RegisterName("Server", c)
	c.server.ServeConn(conn)
}

func (c *ServerConfig) Delta(deltas TextFileDeltas, _ *int) error {
	// make sure all diffs are valid before writing them to disk and cache
	filesToWrite := make([]TextFile, len(deltas))

	for i, delta := range deltas {
		path := delta.Path
		deltaStr := delta.Delta

		diffs, err := dmp.DiffFromDelta(c.fileCache[path], deltaStr)
		if err != nil {
			return err
		}

		newText := dmp.DiffText2(diffs)
		filesToWrite[i] = TextFile{
			Path:    path,
			Content: newText,
		}
	}
	for _, f := range filesToWrite {
		err := afero.WriteFile(c.ServerFs, f.Path, []byte(f.Content), 0644)
		if err != nil {
			return err
		}
		c.fileCache[f.Path] = f.Content
	}
	return nil
}

func (c *ServerConfig) GetFileHashes(_ int, index *ChecksumIndex) error {
	m := make(ChecksumIndex)
	for path, text := range c.fileCache {
		log.Println(path)
		m[path] = crc64checksum(text)
	}
	*index = m
	return nil
}

func (c *ServerConfig) GetTextFile(path string, content *string) error {
	*content = c.fileCache[path]
	return nil
}

// warning: blindly overwrites existing files
func (c *ServerConfig) SendTextFile(file TextFile, _ *int) error {
	//	TODO cache entire file, not just Content (because maybe additional metadata)
	c.fileCache[file.Path] = file.Content
	// TODO store file mode in TextFile struct
	return afero.WriteFile(c.ServerFs, file.Path, []byte(file.Content), 0644)
}

func ServerMain() {
	sourceDir := os.Getenv(EnvSourceDir)
	err := os.Chdir(sourceDir)
	die("could not find server source dir", err)

	// log in server-side sources for convenience
	file, err := os.OpenFile("server.log", os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	die("server side open", err)
	defer file.Close()
	log.SetOutput(file)

	server := NewServerConfig(afero.NewOsFs())
	wd, err := os.Getwd()
	die("get cwd", err)

	server.IgnoreCfg = DefaultIgnoreConfig
	server.path = wd
	server.BuildCache()

	server.ReadCommands(&ReadWriteCloseAdapter{os.Stdout, os.Stdin})
}
