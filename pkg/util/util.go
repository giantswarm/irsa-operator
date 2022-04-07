package util

import "strings"

func RemoveOrg(name string) string {
	return strings.Replace(name, "org-", "", 1)
}
