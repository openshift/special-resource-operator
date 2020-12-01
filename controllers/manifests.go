package controllers

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	errs "github.com/pkg/errors"
)

type assetsFromFile struct {
	name    string
	content []byte
}

func getAssetsFrom(assets string) []assetsFromFile {

	manifests := []assetsFromFile{}
	files, err := filePathWalkDir(assets, ".yaml")
	if err != nil {
		panic(err)
	}
	for _, file := range files {

		buffer, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, assetsFromFile{path.Base(file), buffer})
	}
	return manifests
}

func filePathWalkDir(root string, ext string) ([]string, error) {

	var files []string

	if _, err := os.Stat(root); os.IsNotExist(err) {
		exitOnError(errs.Wrap(err, "Directory does note exists, giving up: "+root))
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		log.Info("WalkDir", "path", path)

		if info.IsDir() {
			log.Info("WalkDir", "path IsDir", path)
			// Ignore root directory but skipdir any subdirectories
			if path == root {
				return nil
			}
			return filepath.SkipDir
		}
		if filepath.Ext(path) != ext {
			log.Info("WalkDir", "path not *.yaml", path)
			return nil
		}
		if result, _ := filepath.Match("[0-9][0-9][0-9][0-9]-*.yaml", filepath.Base(path)); !result {
			log.Info("WalkDir", "path no 0000-", path)
			return nil
		}
		log.Info("WalkDir", "path valid", path)
		files = append(files, path)
		return nil
	})
	return files, err
}
