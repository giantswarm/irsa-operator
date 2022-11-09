package project

var (
	description = "The irsa-operator manages IAM Roles for Service Accounts for aws cluster."
	gitSHA      = "n/a"
	name        = "irsa-operator"
	source      = "https://github.com/giantswarm/irsa-operator"
	version     = "0.8.5"
)

func Description() string {
	return description
}

func GitSHA() string {
	return gitSHA
}

func Name() string {
	return name
}

func Source() string {
	return source
}

func Version() string {
	return version
}
