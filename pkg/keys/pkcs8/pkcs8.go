package pkcs8

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
)

func WritePublicKey(w io.Writer, key *rsa.PrivateKey) error {
	if err := pem.Encode(w, &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	}); err != nil {
		return fmt.Errorf("cannot encode the public key: %w", err)
	}
	return nil
}

func WritePrivateKey(w io.Writer, key *rsa.PrivateKey) error {
	pkcs8PrivateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("cannot marshal the private key to PKCS8: %w", err)
	}
	if err := pem.Encode(w, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkcs8PrivateKey,
	}); err != nil {
		return fmt.Errorf("cannot encode the private key: %w", err)
	}
	return nil
}
