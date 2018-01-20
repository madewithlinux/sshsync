package test

import (
	"os"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
	"github.com/spf13/afero"
)

func WithFolder(t *testing.T, testName string, f func(absPath string, fs afero.Fs)) {
	err := os.Mkdir(testName, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testName)
	absPath, err := filepath.Abs(testName)
	assert.NoError(t, err)
	fs := afero.NewBasePathFs(afero.NewOsFs(), testName)
	f(absPath, fs)
}


func AssertFileContent(t *testing.T, fs afero.Fs, path string, content string) {
	fileBytes, err := afero.ReadFile(fs, path)
	assert.NoError(t, err)
	assert.Equal(t, content, string(fileBytes))
}
