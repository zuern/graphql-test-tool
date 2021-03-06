# Changelog

All changes to the GraphQL Test Tool (gtt) are documented here. Releases follow semantic versioning.

## [Unreleased]

## [1.0.9] - [2019-12-31]

Sort nested arrays

### Fixed

- Nested arrays are now sorted when specified with a `sortBy` step option.

## [1.0.8] - [2019-12-18]

Add dockerfile to repository

### Added
- Dockerfile

## [1.0.7] - [2019-12-18]

Array limits bug fix (v1.0.6 wasn't picked up by go mod, bug in sum.golang.org)

### Fixed

- Check for array comparisons now check for length.

## [1.0.5] - [2019-12-17]

Test Reporting Improvements

### Added

- Displays mismatched values as part of error message when actual is not the same as expected.

## [1.0.2] - [2019-12-16]

Sorting

### Fixed

- Sort before displaying results instead of after.

## [1.0.1] - [2019-12-16]

Reorganize

### Fixed

- Folder and file layout so go.mod works as expected.

- Fixed support for comparing text responses.

## [1.0.0] - [2019-12-15]

Iniital release
