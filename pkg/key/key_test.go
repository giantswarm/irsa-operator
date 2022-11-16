package key

import (
	"testing"

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
