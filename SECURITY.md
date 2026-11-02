# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.1.x   | :white_check_mark: |
| < 2.1   | :x:                |

## Reporting a Vulnerability

Report privately through [GitHub Security Advisories](https://github.com/1mb-dev/obcache-go/security/advisories), not in a public issue. Include steps to reproduce and the impact you see.

We acknowledge reports within 48 hours, aim to resolve critical issues within 7 days, and coordinate disclosure timing with you. There is no bug bounty; with your permission, we credit you in the release notes.

## Using obcache-go Safely

The trust limits are in the README under [Limits](README.md#limits). In short: anyone who can write to your Redis can forge cached values, keys up to 64 bytes hold arguments in clear, and `DebugHandler` has no authentication.

## Tooling

CI runs govulncheck and golangci-lint, including gosec. Dependabot keeps dependencies current.
