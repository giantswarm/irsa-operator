package oidc

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
)

type KeyResponse struct {
	Keys []*Keys `json:"keys"`
}

type Keys struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func digestOfKey(key *rsa.PrivateKey) string {
	s := sha256.New()
	s.Write(x509.MarshalPKCS1PrivateKey(key))
	return base64.RawURLEncoding.EncodeToString(s.Sum(nil))
}

func Write(w io.Writer, key *rsa.PrivateKey) error {
	keyE := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	keyN := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	v := KeyResponse{
		Keys: []*Keys{
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: digestOfKey(key),
				E:   keyE,
				N:   keyN,
			},
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: "",
				E:   keyE,
				N:   keyN,
			},
		},
	}
	if err := json.NewEncoder(w).Encode(&v); err != nil {
		return fmt.Errorf("cannot encode to JSON: %w", err)
	}
	return nil
}
