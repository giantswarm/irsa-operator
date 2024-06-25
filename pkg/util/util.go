package util

import (
	"fmt"
	"reflect"
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

func FilterUniqueTags[T any](tags []T) []T {
	uniqueTags := make(map[string]string)
	filteredTags := make([]T, 0)

	for _, tag := range tags {
		tagValue := reflect.ValueOf(tag)
		keyField := tagValue.FieldByName("Key")
		valueField := tagValue.FieldByName("Value")

		if !keyField.IsValid() || !valueField.IsValid() {
			continue
		}

		key := keyField.Elem().String()
		value := valueField.Elem().String()

		if _, exists := uniqueTags[key]; !exists {
			uniqueTags[key] = value
			filteredTags = append(filteredTags, tag)
		}
	}

	return filteredTags
}
