package oidc

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"

	jose "gopkg.in/square/go-jose.v2"
)

type KeyResponse struct {
	Keys []jose.JSONWebKey `json:"keys"`
}

// copied from kubernetes/kubernetes#78502
func digestOfKey(key *rsa.PrivateKey) (string, error) {
	publicKeyDERBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to serialize public key to DER format: %v", err)
	}

	hasher := crypto.SHA256.New()
	hasher.Write(publicKeyDERBytes)
	publicKeyDERHash := hasher.Sum(nil)

	keyID := base64.RawURLEncoding.EncodeToString(publicKeyDERHash)

	return keyID, nil
}

func GenerateKeysFile(key *rsa.PrivateKey) (*bytes.Reader, error) {
	var alg jose.SignatureAlgorithm

	kid, err := digestOfKey(key)
	if err != nil {
		return nil, err
	}

	var keys []jose.JSONWebKey
	keys = append(keys, jose.JSONWebKey{
		Key:       key.Public(),
		KeyID:     kid,
		Algorithm: string(alg),
		Use:       "sig",
	})

	keyResponse := KeyResponse{Keys: keys}
	byt, err := json.MarshalIndent(keyResponse, "", "    ")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(byt), nil
}
