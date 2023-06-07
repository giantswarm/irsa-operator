package tagsdiff

import (
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

type DiffResult struct {
	Changed bool
	Added   []*iam.Tag
	Removed []*string
}

func Diff(tags []*iam.Tag, desiredTags []*iam.Tag) DiffResult {
	existing := make(map[string]string)

	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			existing[*tag.Key] = *tag.Value
		}
	}

	desired := make(map[string]string)

	for _, tag := range desiredTags {
		if tag.Key != nil && tag.Value != nil {
			desired[*tag.Key] = *tag.Value
		}
	}

	if reflect.DeepEqual(existing, desired) {
		return DiffResult{
			Changed: false,
		}
	}

	added := make([]*iam.Tag, 0)
	for k, v := range desired {
		if _, found := existing[k]; !found {
			added = append(added, &iam.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}

	removed := make([]*string, 0)
	for k := range existing {
		if _, found := desired[k]; !found {
			removed = append(removed, aws.String(k))
		}
	}

	return DiffResult{
		Changed: true,
		Added:   added,
		Removed: removed,
	}
}
