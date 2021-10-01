# Honeycomb Lambda Extension Changelog

## 10.0.1 (2021-10-01)

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