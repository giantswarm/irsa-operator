package tagsdiff

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

func TestEqual(t *testing.T) {
	tests := []struct {
		name        string
		tags        []*iam.Tag
		desiredTags []*iam.Tag
		changed     bool
		added       map[string]string
		removed     []string
	}{
		{
			name:        "Both empty",
			tags:        nil,
			desiredTags: nil,
			changed:     false,
			added:       map[string]string{},
			removed:     []string{},
		},
		{
			name:        "Both with the same value",
			tags:        []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			desiredTags: []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			changed:     false,
			added:       map[string]string{},
			removed:     []string{},
		},
		{
			name:        "One tag missing",
			tags:        nil,
			desiredTags: []*iam.Tag{{Key: aws.String("b"), Value: aws.String("c")}},
			changed:     true,
			added:       map[string]string{"b": "c"},
			removed:     []string{},
		},
		{
			name:        "One additional tag",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			desiredTags: nil,
			changed:     true,
			added:       map[string]string{},
			removed:     []string{"a"},
		},
		{
			name:        "One tag missing",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			desiredTags: []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}, {Key: aws.String("b"), Value: aws.String("c")}},
			changed:     true,
			added:       map[string]string{"b": "c"},
			removed:     []string{},
		},
		{
			name:        "One additional tag",
			tags:        []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}, {Key: aws.String("b"), Value: aws.String("c")}},
			desiredTags: []*iam.Tag{{Key: aws.String("a"), Value: aws.String("b")}},
			changed:     true,
			added:       map[string]string{},
			removed:     []string{"b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Diff(tt.tags, tt.desiredTags)

			if result.Changed != tt.changed {
				t.Errorf("Equal: got changed = %v, wanted %v", result.Changed, tt.changed)
			}

			added := make(map[string]string)
			for _, t := range result.Added {
				added[*t.Key] = *t.Value
			}

			if !reflect.DeepEqual(added, tt.added) {
				t.Errorf("Equal: got added = %v, wanted %v", added, tt.added)
			}

			removed := make([]string, 0)
			for _, t := range result.Removed {
				removed = append(removed, *t)
			}

			if !reflect.DeepEqual(removed, tt.removed) {
				t.Errorf("Equal: got removed = %v, wanted %v", removed, tt.removed)
			}
		})
	}
}
