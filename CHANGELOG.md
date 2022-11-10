# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- Add new service to handle route53 DNS records.

### Changed

- Improve `oidc` service in order to recreate the OIDC provider on AWS when any config is changed.
- Improve `cloudfront` service in order to update the cloudfront distribution on AWS when any config is changed.

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

[Unreleased]: https://github.com/giantswarm/irsa-operator/compare/v0.8.5...HEAD
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
