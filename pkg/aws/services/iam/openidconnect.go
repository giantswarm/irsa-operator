package iam

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
)

const clientID = "sts.amazonaws.com"

func (s *Service) Create(ctx context.Context, identityProviderURL, region string) error {

	s3Endpoint := fmt.Sprintf("s3-%s.amazonaws.com", region)

	tp, err := caThumbPrint(s3Endpoint, "443")
	if err != nil {
		return err
	}

	i := &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(identityProviderURL),
		ThumbprintList: []*string{aws.String(tp)},
		ClientIDList:   []*string{aws.String(clientID)},
	}

	_, err = s.Client.CreateOpenIDConnectProvider(i)
	if err != nil {
		return err
	}

	return nil
}

func caThumbPrint(ep string, port string) (string, error) {
	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%s", ep, port), &tls.Config{
		ServerName: ep,
	})
	if err != nil {
		return "", err
	}

	defer conn.Close()

	// Get the ConnectionState struct as that's the one which gives us x509.Certificate struct
	cert := conn.ConnectionState().PeerCertificates[0]
	fingerprint := md5.Sum(cert.Raw)

	var buf bytes.Buffer
	for i, f := range fingerprint {
		if i > 0 {
			fmt.Fprintf(&buf, ":")
		}
		fmt.Fprintf(&buf, "%02X", f)
	}
	return strings.Replace(buf.String(), ":", "", -1), nil

}
