# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/giantswarm/irsa-operator/compare/v0.23.1...HEAD
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
