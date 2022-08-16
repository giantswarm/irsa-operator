package oidc

import (
	"encoding/json"
	"testing"

	"github.com/blang/semver"
)

func TestGenerateDiscoveryFile(t *testing.T) {
	type args struct {
		domain     string
		bucketName string
		region     string
	}
	tests := []struct {
		name        string
		args        args
		wantIssuer  string
		wantJWKSUri string
		wantErr     bool
	}{
		{
			name: "case 0",
			args: args{
				domain:     "foo.cloudfront.net",
				bucketName: "123456789012-g8s-test1-oidc-pod-identity-v2",
				region:     "eu-west-1",
			},
			wantIssuer:  "https://foo.cloudfront.net",
			wantJWKSUri: "https://foo.cloudfront.net/keys.json",
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release, _ := semver.New("18.0.0")
			got, err := GenerateDiscoveryFile(release, tt.args.domain, tt.args.bucketName, tt.args.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateDiscoveryFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			v := &DiscoveryResponse{}
			if err = json.NewDecoder(got).Decode(&v); err != nil {
				t.Errorf("cannot decode: %v", err)
				return
			}
			if v.Issuer != tt.wantIssuer {
				t.Errorf("Issuer = %v, want %v", v.Issuer, tt.wantIssuer)
			}
			if v.JwksURI != tt.wantJWKSUri {
				t.Errorf("JwksURI = %v, want %v", v.JwksURI, tt.wantJWKSUri)
			}
		})
	}
}
