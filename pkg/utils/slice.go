package utils

import (
	"helm.sh/helm/v3/pkg/chart"
)

// Find returns the smallest index i at which x == a[i],
// or len(a) if there is no such index.
func StringSliceFind(a []string, x string) int {
	for i, n := range a {
		if x == n {
			return i
		}
	}
	return len(a)
}

// Contains tells whether a contains x.
func StringSliceContains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func FindCRFile(a []*chart.File, x string) int {
	for i, n := range a {
		if n.Name == x+".yaml" {
			return i
		}
	}
	return -1
}

func StringSliceInsert(a []string, index int, value string) []string {
	if len(a) == index { // nil or empty slice or after last element
		return append(a, value)
	}
	a = append(a[:index+1], a[index:]...) // index < len(a)
	a[index] = value
	return a
}
