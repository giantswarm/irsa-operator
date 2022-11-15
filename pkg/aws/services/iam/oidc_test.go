package iam

import "testing"

func Test_sliceEqualsIgnoreCase(t *testing.T) {
	one := "one"
	two := "two"
	oneUpper := "One"
	twoUpper := "Two"

	tests := []struct {
		name string
		src  []*string
		dst  []*string
		want bool
	}{
		{
			name: "Both nil",
			src:  nil,
			dst:  nil,
			want: true,
		},
		{
			name: "Same values, same order",
			src:  []*string{&one},
			dst:  []*string{&one},
			want: true,
		},
		{
			name: "Same values, different order",
			src:  []*string{&one, &two},
			dst:  []*string{&two, &one},
			want: true,
		},
		{
			name: "Same values, different order with nils",
			src:  []*string{&one, &two, nil},
			dst:  []*string{&two, nil, &one},
			want: true,
		},
		{
			name: "Same values, different case",
			src:  []*string{&one, &two},
			dst:  []*string{&oneUpper, &twoUpper},
			want: true,
		},
		{
			name: "Different values",
			src:  []*string{&one},
			dst:  []*string{&two},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sliceEqualsIgnoreCase(tt.src, tt.dst); got != tt.want {
				t.Errorf("sliceEqualsIgnoreCase() = %v, want %v", got, tt.want)
			}
		})
	}
}
