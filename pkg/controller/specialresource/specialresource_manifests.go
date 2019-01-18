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

type StageDriverManifests struct {
	serviceAccount []byte
	role           []byte
        roleBinding    []byte
        configMap      []byte
        daemonSet      []byte 
}
type StageDevicePluginManifests struct {
	serviceAccount []byte
	role           []byte
        roleBinding    []byte
        daemonSet      []byte
}
type StageMonitoringManifests struct {
	serviceAccount []byte
}

var stageDriverManifests       StageDriverManifests
var stageDevicePluginManifests StageDevicePluginManifests
var stageMonitoringManifests   StageMonitoringManifests

func GetAssetsFromPath(path string) []assetsFromFile {
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

func GenerateStageDriverManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/stage-driver")
	stageDriverManifests.serviceAccount = manifests[0]
	stageDriverManifests.role           = manifests[1]
	stageDriverManifests.roleBinding    = manifests[2]
	stageDriverManifests.configMap      = manifests[3]
	stageDriverManifests.daemonSet      = manifests[4]
}

func GenerateStageDevicePluginManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/stage-device-plugin")
	stageDriverManifests.serviceAccount = manifests[0]
	stageDriverManifests.role           = manifests[1]
	stageDriverManifests.roleBinding    = manifests[2]
	stageDriverManifests.daemonSet      = manifests[3]
}

func GenerateStageMonitoringManifests() {
	manifests := GetAssetsFromPath("/opt/special-resource-operator/assets/stage-monitoring")
	stageDriverManifests.serviceAccount = manifests[0]
}

