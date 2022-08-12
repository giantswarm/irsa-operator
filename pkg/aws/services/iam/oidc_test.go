package iam

import "testing"

func Test_caThumbPrint(t *testing.T) {
	got, _ := caThumbPrint("d2oruhrymg2w9x.cloudfront.net")
	t.Log(removeColon(got))
}
