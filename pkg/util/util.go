package util

import (
	"reflect"
	"strings"
)

func RemoveOrg(name string) string {
	return strings.Replace(name, "org-", "", 1)
}

// Get keys if maps differ
func MapsDiff(m1, m2 map[string]string) []string {
	equal := reflect.DeepEqual(m1, m2)
	if equal {
		return nil
	}
	var diff []string
	for k := range m2 {
		if _, ok := m1[k]; !ok {
			diff = append(diff, k)
		}
	}
	return diff
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
