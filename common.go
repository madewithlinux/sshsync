package sshsync

import (
	"bytes"
	"github.com/spf13/afero"
	"github.com/gobwas/glob"
	"log"
	"strings"
	"hash/crc64"
	"fmt"
	"io"
	"os"
	"golang.org/x/crypto/ssh"
	"os/exec"
	"io/ioutil"
)

// protocol constants
const (
	// retrieve a file from server
	// format:
	// get_text_file
	// <filename>
	// server response:
	// <number of bytes>
	// <text of file>
	// <newline>
	GetTextFile = "get_text_file"

	// send a text file to the server
	// format:
	// <number of bytes>
	// <raw text file>
	// <newline>
	SendTextFile = "send_text_file"

	// environment constants
	// used by the Client to pass parameters to the server
	// (to avoid shell interpolation)
	// must start with LC_ to go through ssh for some reason
	EnvSourceDir = "LC_SSHSYNC_SOURCE_DIR"
	EnvIgnoreCfg = "LC_SSHSYNC_IGNORE_CFG"

	BinName = "sshsync"
)

// TODO serialize this so it can go in env
type IgnoreConfig struct {
	Extensions []string
	// glob matched
	GlobIgnore         []string
	compiledGlobIgnore []glob.Glob
}

var DefaultIgnoreConfig = IgnoreConfig{
	Extensions: []string{
		".cpp",
		".hpp",
		".c",
		".h",
		".go",
		".hs",
		".cl",
		".js",
		".md",
		".txt",
	},
	GlobIgnore: []string{
		// ignore all hidden files and folders
		".*",
		// ignore build folders
		"build/*",
		"target/*",
		"out/*",
	},
}

// call this before using compiled glob patterns
func (cfg *IgnoreConfig) compileGlobs() {
	if len(cfg.GlobIgnore) == len(cfg.compiledGlobIgnore) {
		return
	}
	cfg.compiledGlobIgnore = make([]glob.Glob, len(cfg.GlobIgnore))
	for i, globIgnoreString := range cfg.GlobIgnore {
		var err error
		cfg.compiledGlobIgnore[i], err = glob.Compile(globIgnoreString)
		if err != nil {
			log.Println("bad glob pattern:", globIgnoreString, err)
			panic("bad glob pattern: " + globIgnoreString + " " + err.Error())
		}
	}
}

func (cfg *IgnoreConfig) ShouldIgnore(fs afero.Fs, path string) bool {
	// if zero-initialized, ignore only what can't be stat
	if len(cfg.Extensions) == 0 &&
		len(cfg.GlobIgnore) == 0 &&
		len(cfg.compiledGlobIgnore) == 0 {
		log.Println("default not ignoring", path)
		stat, err := fs.Stat(path)
		if err != nil && stat.IsDir() {
			log.Println("default ignoring dir", path)
			return true
		} else if err != nil && !stat.IsDir() {
			log.Println("default not ignoring", path)
			return false
		} else {
			log.Println("default ignoring non existent", path)
			return false
		}
	}

	cfg.compileGlobs()
	for _, globIgnore := range cfg.compiledGlobIgnore {
		if globIgnore.Match(path) {
			log.Println("ignoring", path)
			return true
		}
	}

	info, err := fs.Stat(path)
	if err == nil && info.IsDir() {
		// skip checking endings on directories
		log.Println("ignoring dir", path)
		return true
	} else if err != nil {
		// ignore things we can't stat
		log.Println("ignoring non existent", path)
		return true
	}

	// do not ignore whitelisted extensions
	for _, extension := range cfg.Extensions {
		if strings.HasSuffix(path, extension) {
			log.Println("not ignoring", path)
			return false
		}
	}
	// ignore everything else
	log.Println("default ignoring", path)
	return true
}

func die(label string, err error) {
	if err != nil {
		log.Fatal(label, " error: ", err)
	}
}

func lineCount(text string) int {
	// +1 because newline is separator between lines
	return 1 + bytes.Count([]byte(text), []byte("\n"))
}

var ecmaTable = crc64.MakeTable(crc64.ECMA)

func crc64checksum(content string) uint64 {
	return crc64.Checksum([]byte(content), ecmaTable)
}

func crc64string(content string) string {
	return fmt.Sprintf("%0X", crc64checksum(content))
}

/////////////////////////////////////////////////////////

func makeSigner(keyname string) (signer ssh.Signer, err error) {
	fp, err := os.Open(keyname)
	if err != nil {
		return
	}
	defer fp.Close()

	buf, err := ioutil.ReadAll(fp)
	if err != nil {
		return nil, err
	}
	signer, err = ssh.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}
	return
}
func makeKeyring() []ssh.AuthMethod {
	signers := []ssh.Signer{}
	keys := []string{
		os.Getenv("HOME") + "/.ssh/id_rsa",
		os.Getenv("HOME") + "/.ssh/id_dsa",
		os.Getenv("HOME") + "/.ssh/id_ecdsa",
		os.Getenv("HOME") + "/.ssh/id_ed25519",
	}

	for _, keyname := range keys {
		signer, err := makeSigner(keyname)
		if err == nil {
			signers = append(signers, signer)
		}
	}

	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}
}
func OpenSshConnection(serverSidePath, user, address string) (io.ReadWriteCloser, error) {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            makeKeyring(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// FIXME: conn and session are leaked
	// probably not a problem in this use-case because we would close these connections right before exiting
	// the program anyway

	conn, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, err
	}

	session, err := conn.NewSession()
	if err != nil {
		return nil, err
	}

	//err = session.Setenv(EnvSourceDir, serverSidePath)
	//if err != nil {
	//	return nil, err
	//}

	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	fmt.Println("stdin, stdout", stdin, stdout)

	err = session.Start(BinName + " -server")
	if err != nil {
		return nil, err
	}

	_, err = fmt.Fprintln(stdin, serverSidePath)
	if err != nil {
		return nil, err
	}

	return &ReadWriteCloseAdapter{stdout, stdin}, nil
}

func OpenLocalConnection(path string) (io.ReadWriteCloser, error) {
	serverCmd := exec.Command(BinName)
	serverCmd.Dir = path

	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := serverCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = serverCmd.Start()
	if err != nil {
		return nil, err
	}
	return &ReadWriteCloseAdapter{stdout, stdin}, nil
}

type ReadWriteCloseAdapter struct {
	Reader io.Reader
	Writer io.WriteCloser
}

func (s *ReadWriteCloseAdapter) Write(p []byte) (n int, err error) { return s.Writer.Write(p) }
func (s *ReadWriteCloseAdapter) Close() error                      { return s.Writer.Close() }
func (s *ReadWriteCloseAdapter) Read(p []byte) (n int, err error)  { return s.Reader.Read(p) }

type TextFile struct {
	Path    string
	Content string
}

// map of Path to Crc64
type ChecksumIndex map[string]uint64

type TextFileDelta struct {
	Path  string
	Delta string
}
type TextFileDeltas []TextFileDelta

func TwoWayPipe() (io.ReadWriteCloser, io.ReadWriteCloser) {
	// server read, Client write (and vice versa)
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	return &ReadWriteCloseAdapter{cr, cw}, &ReadWriteCloseAdapter{sr, sw}
}
