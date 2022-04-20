# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

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

[Unreleased]: https://github.com/giantswarm/irsa-operator/compare/v0.3.5...HEAD
[0.3.5]: https://github.com/giantswarm/irsa-operator/compare/v0.3.4...v0.3.5
[0.3.4]: https://github.com/giantswarm/irsa-operator/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/giantswarm/irsa-operator/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/giantswarm/irsa-operator/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/giantswarm/irsa-operator/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/giantswarm/irsa-operator/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/giantswarm/irsa-operator/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/giantswarm/irsa-operator/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/irsa-operator/releases/tag/v0.1.0
