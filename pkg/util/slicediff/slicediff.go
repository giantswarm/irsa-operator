package slicediff

import (
	"strings"
)

type DiffResponse struct {
	Added   []string
	Removed []string
}

func (d *DiffResponse) Changed() bool {
	return len(d.Added)+len(d.Removed) != 0
}

func DiffIgnoreCase(src []*string, dst []*string) DiffResponse {
	srcVal := make([]string, 0)
	dstVal := make([]string, 0)

	for _, s := range src {
		if s == nil {
			srcVal = append(srcVal, "")
		} else {
			srcVal = append(srcVal, strings.ToLower(*s))
		}
	}
	for _, d := range dst {
		if d == nil {
			dstVal = append(dstVal, "")
		} else {
			dstVal = append(dstVal, strings.ToLower(*d))
		}
	}

	return DiffResponse{
		Added:   difference(srcVal, dstVal),
		Removed: difference(dstVal, srcVal),
	}
}

// difference returns the elements in `a` that aren't in `b`.
func difference(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}
