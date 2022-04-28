# Honeycomb Lambda Extension Changelog

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
