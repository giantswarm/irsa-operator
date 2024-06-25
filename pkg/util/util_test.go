package util

import (
	"reflect"
	"testing"
)

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

func StringPointer(v string) *string {
	return &v
}

func TestFilterTags(t *testing.T) {
	type Tag struct {
		Key   *string
		Value *string
	}

	tests := []struct {
		name string
		tags []Tag
		want []Tag
	}{
		{
			name: "nil",
			tags: nil,
			want: []Tag{},
		},
		{
			name: "empty",
			tags: []Tag{},
			want: []Tag{},
		},
		{
			name: "normal",
			tags: []Tag{
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
			},
			want: []Tag{
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
			},
		},
		{
			name: "Duplicate Tag",
			tags: []Tag{
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/cluster"),
					Value: StringPointer("test1"),
				},
			},
			want: []Tag{
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/cluster"),
					Value: StringPointer("test1"),
				},
			},
		},
		{
			name: "Multiple Duplicate Tag",
			tags: []Tag{
				{
					Key:   StringPointer("giantswart.io/installation"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/installation"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/cluster"),
					Value: StringPointer("test1"),
				},
			},
			want: []Tag{
				{
					Key:   StringPointer("giantswart.io/installation"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/organization"),
					Value: StringPointer("test"),
				},
				{
					Key:   StringPointer("giantswart.io/cluster"),
					Value: StringPointer("test1"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//compare the output of the function with the expected output
			if got := FilterUniqueTags(tt.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
