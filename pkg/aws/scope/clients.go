package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/version"
	"sigs.k8s.io/cluster-api/util/record"

	"github.com/giantswarm/irsa-operator/pkg/aws"
)

// AWSClients contains all the aws clients used by the scopes
type AWSClients struct {
	S3         *s3.S3
	IAM        *iam.IAM
	Cloudfront *cloudfront.CloudFront
	EKS        *eks.EKS
}

// NewACMClient creates a new ACM API client for a given session
func NewACMClient(session aws.Session, arn string, target runtime.Object) *acm.ACM {
	acmClient := acm.New(session.Session(), &awsclient.Config{
		Credentials: stscreds.NewCredentials(session.Session(), arn),
		// We use ACM for cloud front, and the certs have to be in us east 1 or it won't work.
		// See 'ACMCertificateArn' section in https://docs.aws.amazon.com/cloudfront/latest/APIReference/API_ViewerCertificate.html
		Region: awsclient.String("us-east-1"),
	})
	acmClient.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	acmClient.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return acmClient
}

// NewCloudfrontClient creates a new Cloudfront API client for a given session
func NewCloudfrontClient(session aws.Session, arn string, target runtime.Object) *cloudfront.CloudFront {
	CloudfrontClient := cloudfront.New(session.Session(), &awsclient.Config{Credentials: stscreds.NewCredentials(session.Session(), arn)})
	CloudfrontClient.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	CloudfrontClient.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return CloudfrontClient
}

// NewEKSClient creates a new EKS API client for a given session
func NewEKSClient(session aws.Session, arn string, target runtime.Object) *eks.EKS {
	EKSClient := eks.New(session.Session(), &awsclient.Config{Credentials: stscreds.NewCredentials(session.Session(), arn)})
	EKSClient.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	EKSClient.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return EKSClient
}

// NewRoute53Client creates a new route53 API client for a given session
func NewRoute53Client(session aws.Session, arn string, target runtime.Object) *route53.Route53 {
	route53Client := route53.New(session.Session(), &awsclient.Config{Credentials: stscreds.NewCredentials(session.Session(), arn)})
	route53Client.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	route53Client.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return route53Client
}

// NewS3Client creates a new S3 API client for a given session
func NewS3Client(session aws.Session, arn string, target runtime.Object) *s3.S3 {
	S3Client := s3.New(session.Session(), &awsclient.Config{Credentials: stscreds.NewCredentials(session.Session(), arn)})
	S3Client.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	S3Client.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return S3Client
}

// NewIAMClient creates a new IAM API client for a given session
func NewIAMClient(session aws.Session, arn string, target runtime.Object) *iam.IAM {
	IAMClient := iam.New(session.Session(), &awsclient.Config{Credentials: stscreds.NewCredentials(session.Session(), arn)})
	IAMClient.Handlers.Build.PushFrontNamed(getUserAgentHandler())
	IAMClient.Handlers.Complete.PushBack(recordAWSPermissionsIssue(target))

	return IAMClient
}
func getUserAgentHandler() request.NamedHandler {
	return request.NamedHandler{
		Name: "irsa-operator/user-agent",
		Fn:   request.MakeAddToUserAgentHandler("awscluster", version.Get().String()),
	}
}

func recordAWSPermissionsIssue(target runtime.Object) func(r *request.Request) {
	return func(r *request.Request) {
		if awsErr, ok := r.Error.(awserr.Error); ok {
			switch awsErr.Code() {
			case "AuthFailure", "UnauthorizedOperation", "NoCredentialProviders":
				record.Warnf(target, awsErr.Code(), "Operation %s failed with a credentials or permission issue", r.Operation.Name)
			}
		}
	}
}
