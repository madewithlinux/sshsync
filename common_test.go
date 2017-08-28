package sshsync

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/spf13/afero"
)

func TestIgnoreConfig_ShouldIgnore(t *testing.T) {
	fs := afero.NewMemMapFs()
	ignore1 := &IgnoreConfig{
		Extensions: []string{".test", ".txt"},
		GlobIgnore: []string{".git/*", "ignored/*"},
	}

	fs.Mkdir(".git", 0644)
	fs.Mkdir("ignored", 0644)
	afero.WriteFile(fs, ".git/config.txt", []byte{}, 0644)
	afero.WriteFile(fs, "ignored/test.txt", []byte{}, 0644)
	afero.WriteFile(fs, "the.test", []byte{}, 0644)
	afero.WriteFile(fs, "important file.txt", []byte{}, 0644)

	assert.True(t, ignore1.ShouldIgnore(fs, ".git/config.txt"))
	assert.True(t, ignore1.ShouldIgnore(fs, ".git"))
	assert.True(t, ignore1.ShouldIgnore(fs, "ignored/test.txt"))

	assert.True(t, ignore1.ShouldIgnore(fs, "does not exist.txt"))
	assert.False(t, ignore1.ShouldIgnore(fs, "important file.txt"))
	assert.False(t, ignore1.ShouldIgnore(fs, "the.test"))
}
