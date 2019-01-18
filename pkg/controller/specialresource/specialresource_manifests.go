package specialresource

import (
	"os"
	"path/filepath"
	"io/ioutil"
)

type assetsFromFile []byte
var manifests []assetsFromFile

func FilePathWalkDir(root string) ([]string, error) {
    var files []string
    err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if !info.IsDir() {
            files = append(files, path)
        }
        return nil
    })
    return files, err
}

type StageDriverManifest struct {
	serviceAccount []byte
	role           []byte
        rolebinding    []byte
        configMap      []byte
        daemonSet      []byte 
}
type StageDevicePluginManifest struct {
	serviceAccount []byte
	role           []byte
        rolebinding    []byte
        daemonSet      []byte
}
type StageMonitoringManifest struct {
	serviceAccount []byte
}



var nfdserviceaccount            []byte
var nfdclusterrole               []byte
var nfdclusterrolebinding        []byte
var nfdsecuritycontextconstraint []byte
var nfdconfigmap                 []byte
var nfddaemonset                 []byte

func GenerateManifests() {
	assets := "/opt/lib/cluster-nfd-operator/assets/node-feature-discovery"
	files, err := FilePathWalkDir(assets)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		buffer, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}
		manifests = append(manifests, buffer)
	}
	
	nfdserviceaccount            = manifests[0]
	nfdclusterrole               = manifests[1]
	nfdclusterrolebinding        = manifests[2]
	nfdsecuritycontextconstraint = manifests[3]
	nfdconfigmap                 = manifests[4]
	nfddaemonset                 = manifests[5]
}

