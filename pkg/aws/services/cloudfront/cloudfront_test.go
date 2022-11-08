package cloudfront

import (
	"fmt"
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

func TestService_tagsNeedUpdating(t *testing.T) {
	clusterName := "lbj23"
	clusterNamespace := "giantswarm"
	installation := "wonderland"

	internalTags := map[string]string{
		"giantswarm.io/organization":                         clusterNamespace,
		"giantswarm.io/cluster":                              clusterName,
		fmt.Sprintf("kubernetes.io/cluster/%s", clusterName): "owned",
		"giantswarm.io/installation":                         installation,
	}

	tests := []struct {
		name         string
		existing     *cloudfront.Tags
		customerTags map[string]string
		want         bool
	}{
		{
			name: "Internal tags present, no customer tags",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String(internalTags["giantswarm.io/installation"])},
			}},
			customerTags: map[string]string{},
			want:         false,
		},
		{
			name: "Tags unchanged",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String(internalTags["giantswarm.io/installation"])},
				{Key: aws.String("customertag1"), Value: aws.String("customertagvalue1")},
			}},
			customerTags: map[string]string{
				"customertag1": "customertagvalue1",
			},
			want: false,
		},
		{
			name: "Default tags missing",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
			}},
			customerTags: map[string]string{},
			want:         true,
		},
		{
			name: "Customer tags missing",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String(internalTags["giantswarm.io/installation"])},
			}},
			customerTags: map[string]string{
				"customertag1": "customertagvalue1",
			},
			want: true,
		},
		{
			name: "Customer tags removed",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String(internalTags["giantswarm.io/installation"])},
				{Key: aws.String("customertag1"), Value: aws.String("customertagvalue1")},
			}},
			customerTags: map[string]string{},
			want:         true,
		},
		{
			name: "Customer tags changed",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String(internalTags["giantswarm.io/installation"])},
				{Key: aws.String("customertag1"), Value: aws.String("customertagvalue1")},
			}},
			customerTags: map[string]string{
				"customertag1": "changed",
			},
			want: true,
		},
		{
			name: "Default tags changed",
			existing: &cloudfront.Tags{Items: []*cloudfront.Tag{
				{Key: aws.String("giantswarm.io/organization"), Value: aws.String(internalTags["giantswarm.io/organization"])},
				{Key: aws.String("giantswarm.io/cluster"), Value: aws.String(internalTags["giantswarm.io/cluster"])},
				{Key: aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", internalTags["giantswarm.io/cluster"])), Value: aws.String("owned")},
				{Key: aws.String("giantswarm.io/installation"), Value: aws.String("CHANGED")},
			}},
			customerTags: map[string]string{},
			want:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagsNeedUpdating(tt.existing, internalTags, DistributionConfig{CustomerTags: tt.customerTags}); got != tt.want {
				t.Errorf("tagsNeedUpdating() = %v, want %v", got, tt.want)
			}
		})
	}
}
