package tagsdiff

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		name        string
		tags        []*iam.Tag
		desiredTags []*iam.Tag
		want        bool
	}{
		{
			name:        "Both empty",
			tags:        nil,
			desiredTags: nil,
			want:        true,
		},
		{
			name:        "Both with the same value",
			tags:        []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			desiredTags: []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			want:        true,
		},
		{
			name:        "One tag missing",
			tags:        nil,
			desiredTags: []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			want:        false,
		},
		{
			name:        "One additional tag",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			desiredTags: nil,
			want:        false,
		},
		{
			name:        "One tag missing",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			desiredTags: []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}, {Key: aws.String("b"), Value: aws.String("c")}},
			want:        false,
		},
		{
			name:        "One additional tag",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}, {Key: aws.String("b"), Value: aws.String("c")}},
			desiredTags: []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Equal(tt.tags, tt.desiredTags); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
