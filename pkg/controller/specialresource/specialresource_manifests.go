package specialresource

import (
	"os"
	"path/filepath"
	"io/ioutil"
)

type assetsFromFile []byte
//var manifests []assetsFromFile

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

type StateDriverManifests struct {
	serviceAccount []byte
	role           []byte
        roleBinding    []byte
        configMap      []byte
        daemonSet      []byte 
}
type StateDevicePluginManifests struct {
	serviceAccount []byte
	role           []byte
        roleBinding    []byte
        daemonSet      []byte
}
type StateMonitoringManifests struct {
	serviceAccount []byte
}

var stateDriverManifests       StateDriverManifests
var stateDevicePluginManifests StateDevicePluginManifests
var stateMonitoringManifests   StateMonitoringManifests

func GetAssetsFromPath(path string) []assetsFromFile {

	manifests := []assetsFromFile{}
	assets := path
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
	return manifests
}

func GenerateStateDriverManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/state-driver")
	stateDriverManifests.serviceAccount = manifests[0]
	stateDriverManifests.role           = manifests[1]
	stateDriverManifests.roleBinding    = manifests[2]
	stateDriverManifests.configMap      = manifests[3]
	stateDriverManifests.daemonSet      = manifests[4]
}

func GenerateStateDevicePluginManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/state-device-plugin")
	stateDevicePluginManifests.serviceAccount = manifests[0]
	stateDevicePluginManifests.role           = manifests[1]
	stateDevicePluginManifests.roleBinding    = manifests[2]
	stateDevicePluginManifests.daemonSet      = manifests[3]
}

func GenerateStateMonitoringManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/state-monitoring")
	stateMonitoringManifests.serviceAccount = manifests[0]
}

