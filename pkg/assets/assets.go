package assets

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const namedTemplatePrefix = "_"

// Metadata manifests filename and content
type Metadata struct {
	Name    string
	Content []byte
}

//go:generate mockgen -source=assets.go -package=assets -destination=mock_assets_api.go

type Assets interface {
	GetFrom(assets string) []Metadata
	ValidStateName(path string) bool
	NamedTemplate(path string) bool
}

type assets struct {
	reState *regexp.Regexp
}

func NewAssets() Assets {
	return &assets{
		reState: regexp.MustCompile(`^[0-9]{4}[-_].*\.yaml$`),
	}
}

// GetFrom reads all manifests 0000- from path and returns them
func (a *assets) GetFrom(assets string) []Metadata {
	manifests := []Metadata{}
	files, err := a.filePathWalkDir(assets, ".yaml")
	if err != nil {
		panic(err)
	}
	for _, file := range files {

		buffer, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, Metadata{path.Base(file), buffer})
	}
	return manifests
}

func (a *assets) filePathWalkDir(root string, ext string) ([]string, error) {

	var files []string

	if _, err := os.Stat(root); os.IsNotExist(err) {
		if fmt.Errorf("Directory %s does not exist, giving up: %w ", root, err) != nil {
			return nil, err
		}
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		if info.IsDir() {
			// Ignore root directory but skipdir any subdirectories
			if path == root {
				return nil
			}
			return filepath.SkipDir
		}
		if filepath.Ext(path) != ext {
			return nil
		}

		if valid := a.filePathPatternValid(path); !valid {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func (a *assets) filePathPatternValid(path string) bool {

	patterns := []string{
		"[0-9][0-9][0-9][0-9]-*.yaml",
		"[0-9][0-9][0-9][0-9]_*.yaml",
	}

	for _, pattern := range patterns {
		if result, _ := filepath.Match(pattern, filepath.Base(path)); !result {
			continue
		}
		return true
	}
	return false
}

func (a *assets) ValidStateName(path string) bool {
	return a.reState.MatchString(filepath.Base(path))
}

func (a *assets) NamedTemplate(path string) bool {
	return strings.HasPrefix(filepath.Base(path), namedTemplatePrefix)
}
