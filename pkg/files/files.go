package files

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"

	"github.com/giantswarm/irsa-operator/pkg/files/oidc"
	pkcs8 "github.com/giantswarm/irsa-operator/pkg/files/pkcs"
)

const (
	KeysFilename             = "keys.json"
	DiscoveryFilename        = "discovery.json"
	PublicSignerKeyFilename  = "signer.pub"
	PrivateSignerKeyFilename = "signer.key"
)

func Generate(bucketName, region string) error {
	baseDirName := fmt.Sprintf("/tmp/%s", bucketName)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("cannot generate a key pair: %w", err)
	}

	if err := os.MkdirAll(baseDirName, 0700); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}

	keysFile, err := os.Create(filepath.Join(baseDirName, KeysFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", KeysFilename, err)
	}
	defer keysFile.Close()
	if err := oidc.Write(keysFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", KeysFilename, err)
	}

	discoveryFile, err := os.Create(filepath.Join(baseDirName, DiscoveryFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", DiscoveryFilename, err)
	}
	defer discoveryFile.Close()
	if err := oidc.WriteDiscovery(discoveryFile, bucketName, region); err != nil {
		return fmt.Errorf("cannot write %s: %w", DiscoveryFilename, err)
	}

	publicKeyFile, err := os.Create(filepath.Join(baseDirName, PublicSignerKeyFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", PublicSignerKeyFilename, err)
	}
	defer publicKeyFile.Close()
	if err := pkcs8.WritePublicKey(publicKeyFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", PublicSignerKeyFilename, err)
	}

	privateKeyFile, err := os.Create(filepath.Join(baseDirName, PrivateSignerKeyFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", PrivateSignerKeyFilename, err)
	}
	defer privateKeyFile.Close()
	if err := pkcs8.WritePrivateKey(privateKeyFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", PrivateSignerKeyFilename, err)
	}
	return nil
}

func ReadFile(bucketName, fileName string) ([]byte, error) {
	path := fmt.Sprintf("/tmp/%s/%s", bucketName, fileName)

	file, err := os.ReadFile(path)
	if err != nil {
		return []byte(""), err
	}
	return file, nil
}

func Delete(bucketName string) error {
	path := fmt.Sprintf("/tmp/%s", bucketName)

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("cannot delete %s: %w", path, err)
	}
	return nil
}
