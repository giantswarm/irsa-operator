package util

import (
	"testing"
)

func TestMapsDiff(t *testing.T) {
	m1 := map[string]string{
		"a": "b",
		"c": "d",
	}
	m2 := map[string]string{
		"a": "b",
		"c": "d",
		"e": "f",
		"g": "h",
	}
	diff := MapsDiff(m1, m2)
	if len(diff) != 2 {
		t.Errorf("Expected two diff, got %v", len(diff))
	}
}
