package pkcs

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
)

func generatePublicKey(w io.Writer, key *rsa.PrivateKey) error {
	pkixPublicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("cannot marshal the private key to PKIX: %w", err)
	}
	if err := pem.Encode(w, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pkixPublicKey,
	}); err != nil {
		return fmt.Errorf("cannot encode the public key: %w", err)
	}
	return nil
}

func generatePrivateKey(w io.Writer, key *rsa.PrivateKey) error {
	if err := pem.Encode(w, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}); err != nil {
		return fmt.Errorf("cannot encode the private key: %w", err)
	}
	return nil
}

func GenerateKeys() (string, string, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", nil, fmt.Errorf("cannot generate a key pair: %w", err)
	}

	priv := &bytes.Buffer{}
	pub := &bytes.Buffer{}

	if err := generatePrivateKey(priv, key); err != nil {
		return "", "", nil, fmt.Errorf("cannot generate public key : %s", err)
	}

	if err := generatePublicKey(pub, key); err != nil {
		return "", "", nil, fmt.Errorf("cannot generate public key : %s", err)
	}

	return priv.String(), pub.String(), key, nil
}
