# Honeycomb Lambda Extension Changelog

## v11.5.1 - 2025-06-30

### Fixes

- fix(eventprocessor): delay shutdown to allow lambda app to flush (#162) | @lizthegrey

## v11.5.0 - 2025-06-03

### 💡 Enhancements

- feat: add option to use kms encrypted keys by @JamieDanielson in https://github.com/honeycombio/honeycomb-lambda-extension/pull/159

### 🛠 Maintenance

- chore: LICENSES update for new/updated dependencies included in builds by @robbkidd in https://github.com/honeycombio/honeycomb-lambda-extension/pull/160

## v11.4.0 - 2025-05-02

This release fixes a security vulnerability in Go 1.22.

### Fixes:

- maint: Bump go to 1.24 and some dependencies (#156) | [@Kent Quirk](https://github.com/kentquirk)

## v11.3.0 - 2025-02-19

This release fixes a security vulnerability from standard library in go 1.19.

### Fixes

- fix: bump go version to 1.22 (#154) | [@Yingrong Zhao](https://github.com/VinozzZ)

### Maintenance

- maint(deps): bump github.com/honeycombio/libhoney-go from 1.24.0 to 1.25.0 (#153) | @dependabot
- maint(deps): bump github.com/stretchr/testify from 1.9.0 to 1.10.0 (#152) | @dependabot
- maint(deps): bump github.com/honeycombio/libhoney-go from 1.23.1 to 1.24.0 (#151) | @dependabot

## v11.2.0 - 2024-11-05

### Fixes

- fix: forward the sample rate from the message (#146) | @NLincoln

### Maintenance

- docs: update vulnerability reporting process (#144) | @robbkidd
- maint: add labels to release.yml for auto-generated grouping (#142) | @JamieDanielson
- maint: update codeowners to pipeline-team (#137) | @JamieDanielson
- maint: update codeowners to pipeline (#136) | @JamieDanielson
- maint(deps): bump github.com/honeycombio/libhoney-go from 1.20.0 to 1.23.1 (#147) | @dependabot
- maint(deps): remove reviewers from dependabot.yml (#148) | @codeboten

## v11.1.2 - 2023-10-13

### Maintenance

- maint(deps): bump github.com/sirupsen/logrus from 1.9.0 to 1.9.3 (#133) | dependabot[bot]
- maint(deps): bump github.com/honeycombio/libhoney-go from 1.18.0 to 1.20.0 (#134) | dependabot[bot]
- maint(deps): bump github.com/stretchr/testify from 1.8.2 to 1.8.4 (#130) | dependabot[bot]
- maint(deps): bump github.com/stretchr/testify from 1.8.1 to 1.8.2 (#126) | dependabot[bot]
- maint: Add dependency licenses (#127) | Mike Goldsmith

## v11.1.1 - 2023-02-27

### Maintenance

- maint: Update AWS regions we publish to (#123) | [@MikeGoldsmith](https://github.com/MikeGoldsmith)
- maint: Update CODEOWNERS (#122) | [@vreynolds](https://github.com/vreynolds)
- chore: update dependabot.yml (#120) | [@kentquirk](https://github.com/kentquirk)
- maint: remove duplicate GOARCH in user-agent (#117)| [@robbkidd](https://github.com/robbkidd)
- Bump github.com/stretchr/testify from 1.8.0 to 1.8.1 (#115)
- Bump github.com/honeycombio/libhoney-go from 1.17.0 to 1.18.0 (#116)

## v11.1.0 - 2022-10-24

### Enhancements

- feat: configurable HTTP transport connect timeout (#111) | @danvendia

### Maintenance

- maint: refactor config and simplify main() (#112) | @robbkidd

## v11.0.0 - 2022-10-13

### 💥 Breaking Changes 💥

The extension's layer name has changed to include the project's SemVer release version.
As a result of this name change, each release of the layer will have a new LayerArn and the LayerVersionArn will consistently end in `1`.

```
# Layer Version ARN Pattern
arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension-<ARCH>-<VERSION>:1

# Layer Version ARN Example
arn:aws:lambda:us-east-1:702835727665:layer:honeycomb-lambda-extension-arm64-v11-0-0:1
```

### Maintenance

- maint: release process updates for friendlier ARNs (#103) | [@robbkidd](https://github.com/robbkidd)
- maint: add new project workflow (#105) | [@vreynolds](https://github.com/vreynolds)
- maint: add config for gh release notes organization (#107) | [@JamieDanielson](https://github.com/JamieDanielson)

## [v10.3.0] Layer version 11 - 2022-10-07

### Added

- feat: configurable event batch send timeout (#98) | [@robbkidd](https://github.com/robbkidd)
- add missing default regions (#93) | [@JamieDanielson](https://github.com/JamieDanielson)

### Maintenance

- Bump github.com/sirupsen/logrus from 1.8.1 to 1.9.0 (#90)
- Bump github.com/honeycombio/libhoney-go from 1.15.8 to 1.17.0 (#99)
- maint: add go 1.19 to CI (#96) | [@vreynolds](https://github.com/vreynolds)

## [v10.3.0] Layer version 10 - 2022-07-25

### Added

- Add support for more AWS regions (#88) | [@pkanal](https://github.com/pkanal)

### Maintenance

- Fixes OpenSSL CVE | [@pkanal](https://github.com/pkanal)

## [v10.2.0] Layer version 9 - 2022-04-08

### Added

- Add shutdown reason event for timeouts and failures (#75) | [@danvendia](https://github.com/danvendia)

### Maintenance

- Add go 1.18 to CI (#77) | [@vreynolds](https://github.com/vreynolds)
- Bump github.com/stretchr/testify from 1.7.0 to 1.7.1 (#78)
- Bump github.com/honeycombio/libhoney-go from 1.15.6 to 1.15.8 (#74)

## [v10.1.0] Layer version 8 - 2022-01-07

### Added

- feat: use msgpack encoding when sending telemetry (#72) | [@vreynolds](https://github.com/vreynolds)

### Maintenance

- ci: Add re-triage workflow (#71) | [@vreynolds](https://github.com/vreynolds)
- docs: update layer version (#70) | [@vreynolds](https://github.com/vreynolds)

## [v10.0.3] Layer version 7 - 2021-11-18

### Maintenance

- Put GOARCH into user agent (#65) | [@dstrelau](https://github.com/dstrelau)

## [v10.0.2] Layer version 6 - 2021-11-03

### Maintenance

- bump libhoney-go (#63)
- empower apply-labels action to apply labels (#62)
- Bump github.com/honeycombio/libhoney-go from 1.15.4 to 1.15.5 (#53)
- leave note for future adventurers (#59)
- ci: fix s3 sync job (#57)
- docs: update to latest version in readme (#58)

## [v10.0.1] Layer version 5 - 2021-10-01

### Fixed

Release 10.0.0 had an issue with the published layer.

- fix: aws publish layer directory name must be extensions (#55)
- fix: split publish based on region support (#52)

## 10.0.0 (2021-09-29)

### Added

- Support ARM build (#46)

### Maintenance

- Change maintenance badge to maintained (#44)
- Add Stalebot (#45)
- Add NOTICE (#42)
- Add note about honeycomb_debug env var (#41)
- Update CI config (#40)
- Bump github.com/stretchr/testify from 1.6.1 to 1.7.0 (#28)
- Bump github.com/sirupsen/logrus from 1.7.0 to 1.8.1 (#30)
- Bump github.com/honeycombio/libhoney-go from 1.14.1 to 1.15.4 (#32)

## 9.0.0 (2021-08-25)
### Added
- Debugging mode via environment variable #38

## 8.0.0 (2021-05-17)
### Fixed
- adds some configurable logging for troubleshooting extension behavior (#20)
- parse JSON emitted to STDOUT from Beelines/libhoney and send along as events (#21)

## 7.0.0 (2021-05-14)
### Fixed
- Flush events queue on wake up. (#16)

## Version 4 (2020-12-09)

- The Logs API returns 202 on subscribe. We were checking for 200.

## Version 3 (2020-11-20)

- Remove unnecessary panic when unable to subscribe to logs API
- Add version string to user agent header

## Version 2 (2020-11-12)

- Added option to disable `platform` messages.
- Adding files like `CODEOWNERS`, `CONTRIBUTORS`, etc.

## Version 1 (2020-11-09)

- Pre-release for testing and demos.
