package oidc

import (
	"encoding/json"
	"fmt"
	"io"
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

func WriteDiscovery(w io.Writer, s3BucketName, s3Region string) error {
	// see https://github.com/aws/amazon-eks-pod-identity-webhook/blob/master/SELF_HOSTED_SETUP.md#create-the-oidc-discovery-and-keys-documents
	v := DiscoveryResponse{
		Issuer:                           fmt.Sprintf("https://%s-%s.amazonaws.com", s3BucketName, s3Region),
		JwksURI:                          fmt.Sprintf("https://%s-%s.amazonaws.com/keys.json", s3BucketName, s3Region),
		AuthorizationEndpoint:            "urn:kubernetes:programmatic_authorization",
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ClaimsSupported:                  []string{"sub", "iss"},
	}
	if err := json.NewEncoder(w).Encode(&v); err != nil {
		return fmt.Errorf("cannot encode to JSON: %w", err)
	}
	return nil
}
