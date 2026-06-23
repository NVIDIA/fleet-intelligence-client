# Changelog

## nvfleetctl 0.0.0 (23 Jun 2026)

Initial public development release.

### New Features

- Added the `nvfleetctl` CLI for authenticating with the Fleet Intelligence
  customer API, inspecting compute zones, node groups, and nodes, tracking
  alerts, and generating reports.
- Added the public `pkg/fleetintelligence` Go SDK over the generated OpenAPI
  client.
- Added local config handling for `~/.config/nvfleetctl/config.yaml` with
  restrictive file permissions.
- Added table and JSON output modes, pagination helpers, and shared timeout
  handling.
- Added signed inventory report download and local Sigstore bundle
  verification.

### Project Infrastructure

- Added Apache-2.0 licensing, security reporting guidance, contribution
  guidance, issue templates, a pull request template, and GitHub Actions CI.
- Added OpenAPI client generation through `make generate`.
