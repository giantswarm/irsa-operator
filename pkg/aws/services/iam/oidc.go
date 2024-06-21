package iam

import (
	"bytes"
	"crypto/sha1" //nolint:gosec
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/blang/semver"
	"github.com/giantswarm/microerror"
	"github.com/pkg/errors"

	"github.com/giantswarm/irsa-operator/pkg/key"
	"github.com/giantswarm/irsa-operator/pkg/util"
	"github.com/giantswarm/irsa-operator/pkg/util/slicediff"
	"github.com/giantswarm/irsa-operator/pkg/util/tagsdiff"
)

func (s *Service) EnsureOIDCProviders(identityProviderURLs []string, clientID string, customerTags map[string]string) error {
	providers, err := s.findOIDCProviders()
	if err != nil {
		return microerror.Mask(err)
	}

	thumbprints := make([]*string, 0)
	thumbprintsSeen := make(map[string]bool)
	for _, identityProviderURL := range identityProviderURLs {
		tps, err := caThumbPrints(identityProviderURL)
		if err != nil {
			return err
		}

		// avoid duplicates
		for _, tp := range tps {
			// Avoid pointer aliasing in Go <1.22 by creating a loop-scoped variable. Also ensure same case so we don't
			// get such duplicates.
			tp := strings.ToLower(tp)

			if _, seen := thumbprintsSeen[tp]; !seen {
				thumbprints = append(thumbprints, &tp)
				thumbprintsSeen[tp] = true
			}
		}
	}

	// Ensure there is one provider for each of the URLs
	for _, identityProviderURL := range identityProviderURLs {
		desiredTags := make([]*iam.Tag, 0)
		// Add internal and customer tags.
		{
			for k, v := range s.internalTags() {
				tag := &iam.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				}
				desiredTags = append(desiredTags, tag)
			}

			var tagKeys []string
			for _, item := range desiredTags {
				tagKeys = append(tagKeys, *item.Key)
			}

			for k, v := range customerTags {
				if !util.StringInSlice(k, tagKeys) {
					tag := &iam.Tag{
						Key:   aws.String(k),
						Value: aws.String(v),
					}
					desiredTags = append(desiredTags, tag)
				}
			}

			// Add a tag 'giantswarm.io/alias' that has value true for the provider having predictable URL and false for the cloudfront one
			val := "true"
			if strings.HasSuffix(identityProviderURL, "cloudfront.net") {
				val = "false"
			}
			desiredTags = append(desiredTags, &iam.Tag{
				Key:   aws.String("giantswarm.io/alias"),
				Value: aws.String(val),
			})
		}

		desiredTags = removeDuplicates(desiredTags)

		// Check if one of the providers is already using the right URL.
		found := false
		for arn, existing := range providers {
			if util.EnsureHTTPS(*existing.Url) == util.EnsureHTTPS(identityProviderURL) {
				found = true
				thumbprintsDiff := slicediff.DiffIgnoreCase(existing.ThumbprintList, thumbprints)
				clientidsDiff := slicediff.DiffIgnoreCase(existing.ClientIDList, []*string{&clientID})
				tagsDiff := tagsdiff.Diff(existing.Tags, desiredTags)

				for _, add := range clientidsDiff.Added {
					s.scope.Logger().Info(fmt.Sprintf("Adding client id %s to OIDCProvider for URL %s", add, identityProviderURL))
					_, err = s.Client.AddClientIDToOpenIDConnectProvider(&iam.AddClientIDToOpenIDConnectProviderInput{
						ClientID:                 aws.String(add),
						OpenIDConnectProviderArn: &arn,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Logger().Info(fmt.Sprintf("Added client id %s to OIDCProvider for URL %s", add, identityProviderURL))
				}
				for _, remove := range clientidsDiff.Removed {
					s.scope.Logger().Info(fmt.Sprintf("Removing client id %s to OIDCProvider for URL %s", remove, identityProviderURL))
					_, err = s.Client.RemoveClientIDFromOpenIDConnectProvider(&iam.RemoveClientIDFromOpenIDConnectProviderInput{
						ClientID:                 aws.String(remove),
						OpenIDConnectProviderArn: &arn,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Logger().Info(fmt.Sprintf("Removed client id %s to OIDCProvider for URL %s", remove, identityProviderURL))
				}

				if thumbprintsDiff.Changed() {
					s.scope.Logger().Info(fmt.Sprintf("Updating thumbprints on OIDCProvider for URL %s", identityProviderURL))
					_, err := s.Client.UpdateOpenIDConnectProviderThumbprint(&iam.UpdateOpenIDConnectProviderThumbprintInput{
						OpenIDConnectProviderArn: &arn,
						ThumbprintList:           thumbprints,
					})
					if err != nil {
						return microerror.Mask(err)
					}
					s.scope.Logger().Info(fmt.Sprintf("Updated thumbprints on OIDCProvider for URL %s", identityProviderURL))

				} else {
					s.scope.Logger().Info(fmt.Sprintf("OIDCProvider for URL %s already exists and is up to date", identityProviderURL))
				}

				if tagsDiff.Changed {
					if len(tagsDiff.Added) > 0 {
						s.scope.Logger().Info(fmt.Sprintf("Updating tags on OIDCProvider for URL %s to add %v", identityProviderURL, tagsDiff.Added))
						_, err := s.Client.TagOpenIDConnectProvider(&iam.TagOpenIDConnectProviderInput{
							OpenIDConnectProviderArn: &arn,
							Tags:                     desiredTags,
						})
						if err != nil {
							return microerror.Mask(err)
						}
						s.scope.Logger().Info(fmt.Sprintf("Updated tags on OIDCProvider for URL %s", identityProviderURL))
					}
					if len(tagsDiff.Removed) > 0 {
						s.scope.Logger().Info(fmt.Sprintf("Removing %d undesired tags on OIDCProvider for URL %s", len(tagsDiff.Removed), identityProviderURL))
						_, err := s.Client.UntagOpenIDConnectProvider(&iam.UntagOpenIDConnectProviderInput{
							OpenIDConnectProviderArn: &arn,
							TagKeys:                  tagsDiff.Removed,
						})
						if err != nil {
							return microerror.Mask(err)
						}
						s.scope.Logger().Info(fmt.Sprintf("Removed undesired tags on OIDCProvider for URL %s", identityProviderURL))
					}
				}

				break
			}
		}

		if found {
			continue
		}

		s.scope.Logger().Info(fmt.Sprintf("Creating OIDCProvider for URL %s", identityProviderURL))

		i := &iam.CreateOpenIDConnectProviderInput{
			Url:            aws.String(identityProviderURL),
			ThumbprintList: thumbprints,
			ClientIDList:   []*string{aws.String(clientID)},
			Tags:           desiredTags,
		}

		_, err = s.Client.CreateOpenIDConnectProvider(i)
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Logger().Info(fmt.Sprintf("Created OIDC provider for URL %s", identityProviderURL))
	}
	return nil
}

func (s *Service) internalTags() map[string]string {
	return map[string]string{
		key.S3TagOrganization: util.RemoveOrg(s.scope.ClusterNamespace()),
		key.S3TagCluster:      s.scope.ClusterName(),
		fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName()): "owned",
		key.S3TagInstallation: s.scope.Installation(),
	}
}

func (s *Service) findOIDCProviders() (map[string]*iam.GetOpenIDConnectProviderOutput, error) {
	s.scope.Logger().Info("Looking for existing OIDC providers")
	output, err := s.Client.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	ret := make(map[string]*iam.GetOpenIDConnectProviderOutput, 0)

	for _, providerArn := range output.OpenIDConnectProviderList {
		p, err := s.Client.GetOpenIDConnectProvider(&iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: providerArn.Arn,
		})
		if err != nil {
			return nil, microerror.Mask(err)
		}

		// Check if tags match
		installationTagFound := false
		clusterTagFound := false
		for _, tag := range p.Tags {
			if *tag.Key == key.S3TagInstallation && *tag.Value == s.scope.Installation() {
				installationTagFound = true
			}
			if *tag.Key == key.S3TagCluster && *tag.Value == s.scope.ClusterName() {
				clusterTagFound = true
			}
		}

		if installationTagFound && clusterTagFound {
			ret[*providerArn.Arn] = p
		}
	}

	if len(ret) == 0 {
		s.scope.Logger().Info("Did not find any OIDC provider")
	} else {
		s.scope.Logger().Info(fmt.Sprintf("Found %d existing OIDC providers", len(ret)))
	}

	return ret, nil
}

func (s *Service) ListCustomerOIDCTags(release *semver.Version, cfDomain, accountID, bucketName, region string) (map[string]string, error) {
	var providerArn string
	if (key.IsV18Release(release) && !key.IsChina(region)) || (s.scope.MigrationNeeded() && !key.IsChina(region)) {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", key.ARNPrefix(region), accountID, cfDomain)
	} else {
		providerArn = fmt.Sprintf("arn:%s:iam::%s:oidc-provider/s3.%s.%s/%s", key.ARNPrefix(region), accountID, region, key.AWSEndpoint(region), bucketName)
	}

	i := &iam.ListOpenIDConnectProviderTagsInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
	}

	o, err := s.Client.ListOpenIDConnectProviderTags(i)
	if err != nil {
		return nil, err
	}

	ignoreKeyTags := []string{fmt.Sprintf(key.S3TagCloudProvider, s.scope.ClusterName()), key.S3TagCluster, key.S3TagInstallation, key.S3TagOrganization}
	oidcTags := make(map[string]string)
	for _, tag := range o.Tags {
		if !util.StringInSlice(aws.StringValue(tag.Key), ignoreKeyTags) {
			oidcTags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
		}
	}
	return oidcTags, nil
}

func (s *Service) DeleteOIDCProviders() error {
	providers, err := s.findOIDCProviders()
	if err != nil {
		return microerror.Mask(err)
	}

	for providerArn := range providers {
		i := &iam.DeleteOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(providerArn),
		}

		_, err := s.Client.DeleteOpenIDConnectProvider(i)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case iam.ErrCodeNoSuchEntityException:
					s.scope.Logger().Info("OIDC provider no longer exists, skipping deletion")
					continue
				}
			}
			return err
		}
		s.scope.Logger().Info("Deleted OIDC provider")
	}

	return nil
}

func caThumbPrints(ep string) ([]string, error) {
	ret := make([]string, 0)

	// Root CA certificate.
	{
		client := &http.Client{
			Timeout: time.Second * 10,
		}

		// check PROXY env
		if v, ok := os.LookupEnv("HTTPS_PROXY"); ok {
			proxy, err := url.Parse(v)
			if err != nil {
				return nil, microerror.Mask(err)
			}
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxy)}
		}

		resp, err := client.Get(ep)
		if err != nil {
			return nil, microerror.Mask(errors.Wrapf(err, "failed to get %s", ep))
		}
		defer resp.Body.Close()

		var fingerprint [20]byte
		// Get the latest Root CA from Certificate Chain
		for _, peers := range resp.TLS.PeerCertificates {
			if peers.IsCA {
				fingerprint = sha1.Sum(peers.Raw) //nolint:gosec
			}
		}

		var buf bytes.Buffer
		for _, f := range fingerprint {
			fmt.Fprintf(&buf, "%02X", f)
		}
		ret = append(ret, strings.ToLower(buf.String()))
	}

	return ret, nil
}

func removeDuplicates(tags []*iam.Tag) []*iam.Tag {
	keys := make(map[string]bool)
	list := []*iam.Tag{}

	for _, entry := range tags {
		if _, value := keys[*entry.Key]; !value {
			keys[*entry.Key] = true
			list = append(list, entry)
		}
	}
	return list
}
