package cloudfront

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"k8s.io/klog/klogr"

	"github.com/giantswarm/irsa-operator/pkg/aws/scope"
)

func TestService_distributionNeedsUpdate(t *testing.T) {
	tests := []struct {
		name         string
		distribution *cloudfront.Distribution
		config       DistributionConfig
		want         bool
	}{
		{
			name: "Unchanged",
			distribution: &cloudfront.Distribution{
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases:           nil,
					ViewerCertificate: nil,
				},
			},
			config: DistributionConfig{},
			want:   false,
		},
		{
			name: "Added alias",
			distribution: &cloudfront.Distribution{
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases:           nil,
					ViewerCertificate: nil,
				},
			},
			config: DistributionConfig{
				Aliases: []*string{
					aws.String("test.com"),
				},
			},
			want: true,
		},
		{
			name: "Removed alias",
			distribution: &cloudfront.Distribution{
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Items:    []*string{aws.String("test.com")},
						Quantity: aws.Int64(1),
					},
					ViewerCertificate: nil,
				},
			},
			config: DistributionConfig{
				Aliases: nil,
			},
			want: true,
		},
		{
			name: "Added ACM certificate",
			distribution: &cloudfront.Distribution{
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases:           nil,
					ViewerCertificate: nil,
				},
			},
			config: DistributionConfig{
				CertificateArn: "test",
			},
			want: true,
		},
		{
			name: "Removed ACM certificate",
			distribution: &cloudfront.Distribution{
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: nil,
					ViewerCertificate: &cloudfront.ViewerCertificate{
						ACMCertificateArn: aws.String("test"),
					},
				},
			},
			config: DistributionConfig{
				CertificateArn: "",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterScope := &scope.ClusterScope{
				Logger: klogr.New(),
			}

			s := &Service{
				scope: clusterScope,
			}
			if got := s.distributionNeedsUpdate(tt.distribution, tt.config); got != tt.want {
				t.Errorf("distributionNeedsUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
