# Security Policy

Security fixes go to the latest minor release only.

## Reporting a Vulnerability

Report privately through [GitHub Security Advisories](https://github.com/1mb-dev/obcache-go/security/advisories/new), not in a public issue, with steps to reproduce and the impact you see. Response is best effort, with no fixed times, and we coordinate disclosure with you. There is no bug bounty; with your permission, we credit you in the release notes.

## Out of Scope

Behavior documented under [Limits](README.md#limits) is not a vulnerability. In short: anyone who can write to your Redis can forge cached values, keys up to 64 bytes hold arguments in clear, and `DebugHandler` has no authentication.

## Tooling

CI runs govulncheck and golangci-lint with gosec. Dependabot keeps dependencies current.
