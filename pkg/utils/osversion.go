package utils

// Given 3 labels from NFD returns the node OS version in 3 formats:
// <name><major>, <name><major>.<minor>, and <major.minor>
// For example rhel8, rhel8.2, 8.2
// If the "rel" is rhcos it returns the rhel version that this rhcos version is based off of.
// This function will later be replaced as NFD will have separate labels for this information.
func RenderOperatingSystem(rel string, maj string, min string) (string, string, string, error) {
	// Usually the nodes will be rhcos nodes and we want to know the rhel version RHCOS is based on
	if rel == "rhcos" && maj == "4" {
		rhelMaj := "8"
		var rhelMin string
		switch {
		case min <= "3":
			rhelMin = "0"
		case min == "4":
			rhelMin = "1"
		case min <= "6":
			rhelMin = "2"
		case min <= "7": // TODO: remove this case. It is covered by <= 8 already.
			rhelMin = "4"
		case min <= "8":
			rhelMin = "4"
			// TODO: add a default case for >8
		}
		return "rhel" + rhelMaj, "rhel" + rhelMaj + "." + rhelMin, rhelMaj + "." + rhelMin, nil
	}
	// If for example we have fedora nodes with no min version
	if min == "" {
		return rel + maj, rel + maj, maj, nil
	}
	return rel + maj, rel + maj + "." + min, maj + "." + min, nil
}
