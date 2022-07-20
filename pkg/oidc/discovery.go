package oidc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/blang/semver"

	"github.com/giantswarm/irsa-operator/pkg/key"
)

type DiscoveryResponse struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	JwksURI                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

func GenerateDiscoveryFile(release *semver.Version, domain, bucketName, region string) (*bytes.Reader, error) {
	// see https://github.com/aws/amazon-eks-pod-identity-webhook/blob/master/SELF_HOSTED_SETUP.md#create-the-oidc-discovery-and-keys-documents
	v := DiscoveryResponse{
		AuthorizationEndpoint:            "urn:kubernetes:programmatic_authorization",
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ClaimsSupported:                  []string{"sub", "iss"},
	}
	if key.IsV18Release(release) {
		// Cloudfront
		v.Issuer = fmt.Sprintf("https://%s", domain)
		v.JwksURI = fmt.Sprintf("https://%s/keys.json", domain)
	} else {
		// Public S3 endpoint
		v.Issuer = fmt.Sprintf("https://s3.%s.%s/%s", region, key.AWSEndpoint(region), bucketName)
		v.JwksURI = fmt.Sprintf("https://s3.%s.%s/%s/keys.json", region, key.AWSEndpoint(region), bucketName)
	}

	b := &bytes.Buffer{}

	if err := json.NewEncoder(b).Encode(&v); err != nil {
		return nil, fmt.Errorf("cannot encode to JSON: %w", err)
	}
	return bytes.NewReader(b.Bytes()), nil
}
