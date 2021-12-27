package assets

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/openshift-psap/special-resource-operator/pkg/color"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = zap.New(zap.UseDevMode(true)).WithName(color.Print("manifests", color.Brown))

// Metadata manifests filename and content
type Metadata struct {
	Name    string
	Content []byte
}

//go:generate mockgen -source=assets.go -package=assets -destination=mock_assets_api.go

type Assets interface {
	GetFrom(assets string) []Metadata
	ValidStateName(path string) bool
}

type assets struct {
	log     logr.Logger
	reState *regexp.Regexp
}

func NewAssets() Assets {
	return &assets{
		log:     zap.New(zap.UseDevMode(true)).WithName(color.Print("manifests", color.Brown)),
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
			log.Info("WalkDir", "path IsDir", path)
			// Ignore root directory but skipdir any subdirectories
			if path == root {
				return nil
			}
			return filepath.SkipDir
		}
		if filepath.Ext(path) != ext {
			log.Info("WalkDir", "path does not match *.yaml", path)
			return nil
		}

		if valid := a.filePathPatternValid(path); !valid {
			return nil
		}

		log.Info("WalkDir", "path valid", path)
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
