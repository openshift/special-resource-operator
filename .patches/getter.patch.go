package getter

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var fileProvider = Provider{
	Schemes: []string{"file"},
	New:     NewFileGetter,
}

type FileGetter struct {
}

func (g *FileGetter) Get(href string, option ...Option) (*bytes.Buffer, error) {

	ref := strings.TrimPrefix(href, "file://")

	if _, err := os.Stat(ref); err == nil {
		file, err := ioutil.ReadFile(ref)
		if err == nil {
			return bytes.NewBuffer(file), nil
		}

	} else if os.IsNotExist(err) {
		// path/to/whatever does *not* exist
		fmt.Printf("getter.go: ERROR FILE DOES NOT EXISTS %+v\n", err)
		os.Exit(1)

	} else {
		// Schrodinger: file may or may not exist. See err for details.
		// Therefore, do *NOT* use !os.IsNotExist(err) to test for file existence
		fmt.Printf("ERROR SCHROEDINGER FILE  %+v\n", err)
		os.Exit(1)
	}

	return nil, nil
}

func NewFileGetter(options ...Option) (Getter, error) {
	var client FileGetter

	return &client, nil
}
