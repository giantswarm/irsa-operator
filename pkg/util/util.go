package util

import (
	"fmt"
	"strings"
)

func EnsureHTTPS(url string) string {
	if strings.HasPrefix(url, "https://") {
		return url
	}

	url = strings.TrimPrefix(url, "http://")

	return fmt.Sprintf("https://%s", url)
}

func TrimHTTPS(url string) string {
	if strings.HasPrefix(url, "https://") {
		return strings.TrimPrefix(url, "https://")
	}

	return url
}

func RemoveOrg(name string) string {
	return strings.Replace(name, "org-", "", 1)
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
