# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.33.2] - 2025-06-25

### Fixed

- Don't fail when reconciling cluster deletion if `cluster-values` configmap is gone while trying to remove its finalizer.

## [0.33.1] - 2025-06-11

### Changed

- Remove CM finalizer

## [0.33.0] - 2025-06-10

### Changed

- Remove finalizer when "giantswarm.io/pause-irsa-operator" annotation is present and cluster is deleted. Otherwise the finalizer is never removed. Crossplane will delete cloud resources.


## [0.32.0] - 2025-06-06

### Added

- Add pause annotation "giantswarm.io/pause-irsa-operator" independent from Cluster API to allow Crossplane migration.

### Fixed

- Small fixes for golangci-lint v2
- Updated dependencies to resolve CVEs:
  - golang.org/x/crypto **v0.35.0** --> **v0.37.0**
  - golang.org/x/net **v0.36.0** --> **v0.39.0**
  - golang.org/x/sys **v0.30.0** --> **v0.32.0**
  - golang.org/x/term **v0.29.0** --> **v0.31.0**

## [0.31.1] - 2025-04-03

### Fixed

- When creating the MC OIDC provider on the WC account, use the S3 URL when in China regions.

## [0.31.0] - 2025-03-20

### Changed

- Create Management Cluster OIDC provider on the AWS account used by the workload cluster.

## [0.30.0] - 2024-09-02

### Added

- Conditionally delete CloudFront domain OIDC provider `<random>.cloudfront.net` for vintage AWS clusters based on `AWSCluster` annotation `alpha.aws.giantswarm.io/irsa-keep-cloudfront-oidc-provider={true,false}`

## [0.29.4] - 2024-08-21

### Fixed

- Disable logger development mode to avoid panicking

## [0.29.3] - 2024-08-06

### Fixed

- Fix panics in logging statements

## [0.29.2] - 2024-07-23

### Fixed

- Fix panics in logging statements

## [0.29.1] - 2024-07-23

### Fixed

- Vintage AWS: Consider `giantswarm.io/keep-irsa` label on the `AWSCluster` object. Previously, we checked on the `Cluster` object, but if that was already independently deleted during a cluster migration, a bug led to deleting the IRSA cloud resources (incl. OIDC provider). The [cluster migration CLI now automatically puts this label on the `AWSCluster` object](https://github.com/giantswarm/capi-migration-cli/pull/119).

### Changed

- Upgrade Kubernetes, CAPI and logging modules

## [0.29.0] - 2024-07-04

### Added

- Vintage AWS: Prevent deletion of IRSA related components with `giantswarm.io/keep-irsa` label.

### Fixed

- CAPA clusters will create a new cloudfront distribution when migrating clusters.
- Use different S3 buckets bewteen CAPA and Vintage.

## [0.28.0] - 2024-06-25

### Added

- Add option to configure controller concurrency for CAPA and EKS.

### Fixed

- Fix ConfigMap not found errors after deletion is done.

## [0.27.7] - 2024-06-25

### Fix

- Updated `FilterUniqueTags` function to handle array pointers

## [0.27.6] - 2024-06-25

### Fix

- Updated `FilterUniqueTags` function to handle AWS Tags with pointer fields

## [0.27.5] - 2024-06-25

### Changed

- Avoid duplicate AWS tags

## [0.27.4] - 2024-06-20

### Fixed

- Increase backoff total time to 75 seconds.
- Add backoff when getting validation CNAME.
- Fix secret update error.

## [0.27.3] - 2024-06-19

### Fixed

- Don't try to reconcile EKS clusters that don't exist anymore on the k8s API.

## [0.27.2] - 2024-06-06

### Fixed

- Do not reconcile CloudFront distribution for China region

## [0.27.1] - 2024-04-17

### Changed

- Add taint toleration.
- Add node affinity to prefer scheduling CAPI pods to control-plane nodes.

## [0.27.0] - 2024-04-10

### Added

- Add metric `irsa_operator_acm_certificate_not_after` metric to expose the `NotAfter` timestamp of the ACM certificate.

### Changed

- Add a cache to ACM service to avoid hitting the API too hard.

### Fixed

- Vintage: fix not performing validation on renewal of certificate.

## [0.26.3] - 2024-04-10

### Fixed

- Add switch for the PodMonitor

## [0.26.2] - 2024-04-02

### Fixed

- Use PodMonitor instead of legacy labels.

## [0.26.1] - 2024-03-20

### Fixed

- CAPA: fix not performing validation on renewal of certificate.

## [0.26.0] - 2024-03-19

### Changed

- CAPA: check for deletion timestamp on the Cluster CR.
- CAPA: always check if certificate should be validated

## [0.25.0] - 2024-02-13

### Fixed

- Fix update of OIDC provider thumbprint list with root CA.

## [0.24.1] - 2024-01-30

### Changed

- Avoid unnecessary `ChangeResourceRecordSets` upsert requests if the same update was recently done. This further avoids hitting the AWS Route53 rate limit.

## [0.24.0] - 2024-01-30

### Changed

- List many hosted zones at once in one Route53 request and cache all returned zones. This reduces the number of Route53 requests and therefore avoids rate limit (throttling) errors.
- CAPA: Skip reconciliation if paused annotation exists on `AWSCluster` object

## [0.23.2] - 2024-01-29

### Changed

- Fetch service account secret much later in the process instead of waiting. That way, other resources can be created in the meantime. Also, requeue a reconciliation sooner as the secret may be available before the previous default of "5 minutes later".

## [0.23.1] - 2024-01-17

### Changed

- Retry patching AWSCluster when removing finalizer if it fails the first time

## [0.23.0] - 2024-01-15

### Changed

- Configure `gsoci.azurecr.io` as the default container image registry.
- Removed OIDC provider creation for CF Domain.

## [0.22.0] - 2023-11-08

### Changed

- Removed duplicated tags before creating the OIDC provider.

## [0.21.0] - 2023-11-07

### Added

- Add `global.podSecurityStandards.enforced` value for PSS migration.
- Add AWS tags to all created resources for CAPA and EKS clusters.

## [0.20.0] - 2023-09-21

### Changed

- Update Go dependencies to fix vulnerability in `golang.org/x/net v0.9.0`
- Use `irsa.<baseDomain>` alias for all CAPA clusters including proxy based clusters.

## [0.19.0] - 2023-08-03

### Added

- Add support for EKS CAPI clusters.

## [0.18.0] - 2023-08-01

### Fixed

- Filter hosted zone by zone name when trying to find hosted zone ID.

### Changed

- Build chart using `app-build-suite`.
- Avoid blocking the reconciliation loop when deleting the cloudfront distribution.

## [0.17.1] - 2023-07-17

### Fixed

- Fixed typo.

## [0.17.0] - 2023-07-17

### Changed

- Force SSL access to bucket contents to improve security.

## [0.16.0] - 2023-07-13

### Added

- Added required values for pss policies.

## [0.15.0] - 2023-05-22

### Fixed

- Enable s3 bucket ACL.
- Enable public access for S3 in china.

## [0.14.1] - 2023-05-18

### Fixed

- Fix problem with duplicated tags.

## [0.14.0] - 2023-05-18

### Added

- Add 'giantswarm.io/alias' tag to OIDC provider to distinguish between cloudfront and alias domain providers.

## [0.13.0] - 2023-04-25

### Added

- Allow using Cloudfront Alias before v19.0.0 via annotation `alpha.aws.giantswarm.io/enable-cloudfront-alias`.

## [0.12.1] - 2023-04-20

### Fixed

- Fix pagination when fetching ACM certificates to delete.

## [0.12.0] - 2023-04-17

### Fixed

- CAPA: Keep finalizer on cluster values ConfigMap since we need it to get the base domain. This fixes stuck deletion if the config map was already gone.

## [0.11.2] - 2023-03-15

### Fixed

- Create CNAME record in the private DNS zone as well in legacy.

## [0.11.1] - 2023-03-09

### Fixed

- Avoid setting domainAlias in the IRSA configmap for v18 clusters.

## [0.11.0] - 2023-02-22

### Fixed

- Fix hardcoding release version to 20.0.0-alpha1 for CAPI clusters to ensure the correct bucket name is used. In 0.10.0, this did not work and by mistake, another bucket with the old naming was created and reconciled.

## [0.10.0] - 2023-02-21

### Changed

- Hardcode release version to v20.0.0-alpha1 on CAPI clusters, so that CAPI clusters can remove the release version label.

## [0.9.2] - 2023-02-17

### Fixed

- Use Proxy if set when getting OIDC CAThumbprint from Root CA.

## [0.9.1] - 2023-02-15

### Changed

- Use patch instead of update method for adding/removing finalizer
- Add finalizer before reconciling
- CAPA: Avoid deletion reconciliation if finalizer is already gone (busy loop)
- CAPA: Look up `AWSClusterRoleIdentity` by the correct reference field instead of assuming it is named like the cluster or dangerously falling back to `default`
- Add timeout for getting TLS connection to identity provider

## [0.9.0] - 2023-02-08

### Added

- Add new service to handle route53 DNS records.
- Add new service to handle ACM certificates.
- Use predictable domain alias for cloudfront on legacy clusters.
- Use runtime/default seccomp profile.

### Changed

- Improve `oidc` service in order to recreate the OIDC provider on AWS when any config is changed.
- Improve `cloudfront` service in order to update the cloudfront distribution on AWS when any config is changed.
- Allow having multiple URLs in the `oidc` service.
- Switch to capa `v1beta1`.
- Use both root CA and leaf certificate thumbprints rather than leaf certificate one only.
- Modify the PSP to allow projected and secret volumes.

## [0.8.5] - 2022-11-09

### Fixed

- Limit retries
- Send metrics in case S3 objects cannot be uploaded.
- Add `irsa-operator` to capa-app-collection.
- Fix detection of v19 and v18 releases.

## [0.8.4] - 2022-11-02

### Fixed

- Check if Cloudfront Distribution is empty.
- Unsupported `strings.Title`.

## [0.8.3] - 2022-10-28

### Fixed

- Check that VPA is installed when trying to add VPA resource

## [0.8.2] - 2022-10-14

### Fixed

- Adjusting values for toggle.

## [0.8.1] - 2022-10-14

### Fixed

- K8s event for bootstrap being complete.

## [0.8.0] - 2022-10-14

### Added

- IRSA for CAPA.

## [0.7.0] - 2022-08-18

### Added

- Handle migration from v1 to v2.

## [0.6.0] - 2022-08-17

### Changed

- Cloudfront integration to use private S3 buckets only.

## [0.5.0] - 2022-06-10

### Changed

- Align issuer and jwks_uri in OIDC discovery.

## [0.4.5] - 2022-06-01

### Fixed

- Scraping.

## [0.4.4] - 2022-05-11

### Fixed

- Remove metrics when cluster is deleted.

## [0.4.3] - 2022-05-10

### Fixed

- Allow ingress traffic for monitoring port.

## [0.4.2] - 2022-05-09

### Fixed

- Allow patching events.

## [0.4.1] - 2022-05-09

### Fixed

- Fixed annotations for scraping metrics.

## [0.4.0] - 2022-05-09

### Added

- Prometheus metrics for `irsa-operator`.
- Event recorder for `irsa-operator`.

## [0.3.6] - 2022-04-20

### Fixed

- ARN prefix for region.

## [0.3.5] - 2022-04-20

### Fixed

- Identity URL for OIDC.

## [0.3.4] - 2022-04-20

### Fixed

- AWS Region Endpoint for IRSA.

## [0.3.3] - 2022-04-19

### Fixed

- Fix `ParsePKCS1PrivateKey`.

## [0.3.2] - 2022-04-18

### Fixed

- Calculation for `kid`.

## [0.3.1] - 2022-04-15

## [0.3.0] - 2022-04-13

### Added

- Add giantswarm tags to OIDC S3 bucket.
- Enable encryption for OIDC S3 bucket.
- Support Customer tags.

## [0.2.0] - 2022-03-31

### Changed

- Remove writing resources to files.
- Refactor code so each part can be retried if one of the steps fails.
- Increase request and limits for the deployment pod.
- Upgrade `apiextensions` to `v6.0.0`.

### Added

- Add `capa-controller` to reconcile Cluster API Provider AWS CR's.

## [0.1.1] - 2022-03-09

### Added

- Add `irsa-operator` to AWS app collection.

## [0.1.0] - 2022-03-04

[Unreleased]: https://github.com/giantswarm/irsa-operator/compare/v0.33.2...HEAD
[0.33.2]: https://github.com/giantswarm/irsa-operator/compare/v0.33.1...v0.33.2
[0.33.1]: https://github.com/giantswarm/irsa-operator/compare/v0.33.0...v0.33.1
[0.33.0]: https://github.com/giantswarm/irsa-operator/compare/v0.32.0...v0.33.0
[0.32.0]: https://github.com/giantswarm/irsa-operator/compare/v0.31.1...v0.32.0
[0.31.1]: https://github.com/giantswarm/irsa-operator/compare/v0.31.0...v0.31.1
[0.31.0]: https://github.com/giantswarm/irsa-operator/compare/v0.30.0...v0.31.0
[0.30.0]: https://github.com/giantswarm/irsa-operator/compare/v0.29.4...v0.30.0
[0.29.4]: https://github.com/giantswarm/irsa-operator/compare/v0.29.3...v0.29.4
[0.29.3]: https://github.com/giantswarm/irsa-operator/compare/v0.29.2...v0.29.3
[0.29.2]: https://github.com/giantswarm/irsa-operator/compare/v0.29.1...v0.29.2
[0.29.1]: https://github.com/giantswarm/irsa-operator/compare/v0.29.0...v0.29.1
[0.29.0]: https://github.com/giantswarm/irsa-operator/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/giantswarm/irsa-operator/compare/v0.27.7...v0.28.0
[0.27.7]: https://github.com/giantswarm/irsa-operator/compare/v0.27.6...v0.27.7
[0.27.6]: https://github.com/giantswarm/irsa-operator/compare/v0.27.5...v0.27.6
[0.27.5]: https://github.com/giantswarm/irsa-operator/compare/v0.27.4...v0.27.5
[0.27.4]: https://github.com/giantswarm/irsa-operator/compare/v0.27.3...v0.27.4
[0.27.3]: https://github.com/giantswarm/irsa-operator/compare/v0.27.2...v0.27.3
[0.27.2]: https://github.com/giantswarm/irsa-operator/compare/v0.27.1...v0.27.2
[0.27.1]: https://github.com/giantswarm/irsa-operator/compare/v0.27.0...v0.27.1
[0.27.0]: https://github.com/giantswarm/irsa-operator/compare/v0.26.3...v0.27.0
[0.26.3]: https://github.com/giantswarm/irsa-operator/compare/v0.26.2...v0.26.3
[0.26.2]: https://github.com/giantswarm/irsa-operator/compare/v0.26.1...v0.26.2
[0.26.1]: https://github.com/giantswarm/irsa-operator/compare/v0.26.0...v0.26.1
[0.26.0]: https://github.com/giantswarm/irsa-operator/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/giantswarm/irsa-operator/compare/v0.24.1...v0.25.0
[0.24.1]: https://github.com/giantswarm/irsa-operator/compare/v0.24.0...v0.24.1
[0.24.0]: https://github.com/giantswarm/irsa-operator/compare/v0.23.2...v0.24.0
[0.23.2]: https://github.com/giantswarm/irsa-operator/compare/v0.23.1...v0.23.2
[0.23.1]: https://github.com/giantswarm/irsa-operator/compare/v0.23.0...v0.23.1
[0.23.0]: https://github.com/giantswarm/irsa-operator/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/giantswarm/irsa-operator/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/giantswarm/irsa-operator/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/giantswarm/irsa-operator/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/giantswarm/irsa-operator/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/giantswarm/irsa-operator/compare/v0.17.1...v0.18.0
[0.17.1]: https://github.com/giantswarm/irsa-operator/compare/v0.17.0...v0.17.1
[0.17.0]: https://github.com/giantswarm/irsa-operator/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/giantswarm/irsa-operator/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/giantswarm/irsa-operator/compare/v0.14.1...v0.15.0
[0.14.1]: https://github.com/giantswarm/irsa-operator/compare/v0.14.0...v0.14.1
[0.14.0]: https://github.com/giantswarm/irsa-operator/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/giantswarm/irsa-operator/compare/v0.12.1...v0.13.0
[0.12.1]: https://github.com/giantswarm/irsa-operator/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/giantswarm/irsa-operator/compare/v0.11.2...v0.12.0
[0.11.2]: https://github.com/giantswarm/irsa-operator/compare/v0.11.1...v0.11.2
[0.11.1]: https://github.com/giantswarm/irsa-operator/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/giantswarm/irsa-operator/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/giantswarm/irsa-operator/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/giantswarm/irsa-operator/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/giantswarm/irsa-operator/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/giantswarm/irsa-operator/compare/v0.8.5...v0.9.0
[0.8.5]: https://github.com/giantswarm/irsa-operator/compare/v0.8.4...v0.8.5
[0.8.4]: https://github.com/giantswarm/irsa-operator/compare/v0.8.3...v0.8.4
[0.8.3]: https://github.com/giantswarm/irsa-operator/compare/v0.8.2...v0.8.3
[0.8.2]: https://github.com/giantswarm/irsa-operator/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/giantswarm/irsa-operator/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/giantswarm/irsa-operator/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/giantswarm/irsa-operator/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/giantswarm/irsa-operator/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/giantswarm/irsa-operator/compare/v0.4.5...v0.5.0
[0.4.5]: https://github.com/giantswarm/irsa-operator/compare/v0.4.4...v0.4.5
[0.4.4]: https://github.com/giantswarm/irsa-operator/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/giantswarm/irsa-operator/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/giantswarm/irsa-operator/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/giantswarm/irsa-operator/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/giantswarm/irsa-operator/compare/v0.3.6...v0.4.0
[0.3.6]: https://github.com/giantswarm/irsa-operator/compare/v0.3.5...v0.3.6
[0.3.5]: https://github.com/giantswarm/irsa-operator/compare/v0.3.4...v0.3.5
[0.3.4]: https://github.com/giantswarm/irsa-operator/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/giantswarm/irsa-operator/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/giantswarm/irsa-operator/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/giantswarm/irsa-operator/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/giantswarm/irsa-operator/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/giantswarm/irsa-operator/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/giantswarm/irsa-operator/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/irsa-operator/releases/tag/v0.1.0
