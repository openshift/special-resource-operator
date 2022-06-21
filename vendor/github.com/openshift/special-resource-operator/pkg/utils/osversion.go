package utils

import (
	"fmt"
	"regexp"
)

var (
	versionRegex = regexp.MustCompile(`(?P<ClusterVersion>\d{2,})\.(?P<OSVersion>\d{2,})\.\d{12}-\d`)
)

// ParseOSInfo takes a pretty format string for the OS version (e.g. Red Hat Enterprise Linux CoreOS 49.84.202201102104-0 (Ootpa))
// and returns the cluster version, OS version in \d\.\d format, plus major version for the OS.
func ParseOSInfo(osImagePretty string) (string, string, string, error) {
	clusterVersion, osVersion, osMajor := "", "", ""
	matches := versionRegex.FindStringSubmatch(osImagePretty)
	if len(matches) != 3 {
		return "", "", "", fmt.Errorf("failed to find a match for %s", osImagePretty)
	}
	clusterVersion = fmt.Sprintf("%c.%s", matches[1][0], matches[1][1:])
	osVersion = fmt.Sprintf("%c.%s", matches[2][0], matches[2][1:])
	osMajor = string(matches[2][0])
	return clusterVersion, osVersion, osMajor, nil
}
