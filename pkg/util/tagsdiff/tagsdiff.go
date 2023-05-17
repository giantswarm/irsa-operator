package tagsdiff

import (
	"reflect"

	"github.com/aws/aws-sdk-go/service/iam"
)

func Equal(tags []*iam.Tag, desiredTags []*iam.Tag) bool {
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

	return reflect.DeepEqual(existing, desired)
}
