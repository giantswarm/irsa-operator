package files

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/giantswarm/irsa-operator/pkg/files/oidc"
	"github.com/giantswarm/irsa-operator/pkg/files/pkcs8"
)

const (
	keysFilename       = "keys.json"
	discoveryFilename  = "discovery.json"
	publicKeyFilename  = "signer.pub"
	privateKeyFilename = "signer.key"
)

func Generate(bucketName, region string) error {
	log.Printf("generating a key pair")
	baseDirName := fmt.Sprintf("/tmp/%s", bucketName)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("cannot generate a key pair: %w", err)
	}

	if err := os.MkdirAll(baseDirName, 0700); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}
	log.Printf("created directory %s", baseDirName)

	keysFile, err := os.Create(filepath.Join(baseDirName, keysFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", keysFilename, err)
	}
	defer keysFile.Close()
	if err := oidc.Write(keysFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", keysFilename, err)
	}
	log.Printf("created %s", keysFilename)

	discoveryFile, err := os.Create(filepath.Join(baseDirName, discoveryFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", discoveryFilename, err)
	}
	defer discoveryFile.Close()
	if err := oidc.WriteDiscovery(discoveryFile, bucketName, region); err != nil {
		return fmt.Errorf("cannot write %s: %w", discoveryFilename, err)
	}
	log.Printf("created %s", discoveryFilename)

	publicKeyFile, err := os.Create(filepath.Join(baseDirName, publicKeyFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", publicKeyFilename, err)
	}
	defer publicKeyFile.Close()
	if err := pkcs8.WritePublicKey(publicKeyFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", publicKeyFilename, err)
	}
	log.Printf("created %s", publicKeyFilename)

	privateKeyFile, err := os.Create(filepath.Join(baseDirName, privateKeyFilename))
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", privateKeyFilename, err)
	}
	defer privateKeyFile.Close()
	if err := pkcs8.WritePrivateKey(privateKeyFile, key); err != nil {
		return fmt.Errorf("cannot write %s: %w", privateKeyFilename, err)
	}
	log.Printf("created %s", privateKeyFilename)
	return nil
}
