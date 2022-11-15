package util

import "testing"

func TestEnsureHTTPS(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "Has https",
			url:  "https://test.io",
			want: "https://test.io",
		},
		{
			name: "Has http",
			url:  "http://test.io",
			want: "https://test.io",
		},
		{
			name: "base dns name",
			url:  "test.io",
			want: "https://test.io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnsureHTTPS(tt.url); got != tt.want {
				t.Errorf("EnsureHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}
