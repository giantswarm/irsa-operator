package key

import (
	"testing"

	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v6/pkg/apis/infrastructure/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestBaseDomain(t *testing.T) {
	tests := []struct {
		name    string
		cluster v1beta1.Cluster
		want    string
		wantErr bool
	}{
		{
			name: "Normal API endpoint",
			cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ControlPlaneEndpoint: v1beta1.APIEndpoint{
						Host: "api.blah.com",
					},
				},
			},
			want:    "blah.com",
			wantErr: false,
		},
		{
			name: "Missing API endpoint",
			cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ControlPlaneEndpoint: v1beta1.APIEndpoint{
						Host: "",
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "API endpoint not starting with api.",
			cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ControlPlaneEndpoint: v1beta1.APIEndpoint{
						Host: "something.test.com",
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseDomain(tt.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("BaseDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BaseDomain() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKeepOnDeletion(t *testing.T) {
	tests := []struct {
		name       string
		awsCluster *infrastructurev1alpha3.AWSCluster
		wantToKeep bool
	}{
		{
			name: "No labels",
			awsCluster: &infrastructurev1alpha3.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"hey": "ho",
					},
				},
			},
			wantToKeep: false,
		},
		{
			name: "Other label",
			awsCluster: &infrastructurev1alpha3.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"hey": "ho",
					},
				},
			},
			wantToKeep: false,
		},
		{
			name: "With 'keep' label",
			awsCluster: &infrastructurev1alpha3.AWSCluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"hey":                     "ho",
						"giantswarm.io/keep-irsa": "",
					},
				},
			},
			wantToKeep: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KeepOnDeletion(tt.awsCluster)
			if got != tt.wantToKeep {
				t.Errorf("KeepOnDeletion() got = %v, want %v", got, tt.wantToKeep)
			}
		})
	}
}
