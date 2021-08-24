# Honeycomb Lambda Extension Changelog

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