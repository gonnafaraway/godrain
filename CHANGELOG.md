# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial public open-source packaging (license, CI, contributing docs)

## [0.1.0] - 2026-10-11

### Added

- `godrain watch` — sample live pprof heap profiles and classify retained growth
- `godrain run` — launch a target process, wait for pprof, then watch
- `godrain diff` — offline comparison of two heap profiles
- Classification signals: slope, plateau, allowlist, optional load correlation
- Example allowlist and leaky / cached demo apps
- `godrain version` command

[Unreleased]: https://github.com/gonnafaraway/godrain/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/gonnafaraway/godrain/releases/tag/v0.1.0
