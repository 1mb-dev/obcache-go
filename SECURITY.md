# Security Policy

## Supported Versions

We support the latest version of obcache-go with security updates. Once the project reaches v1.0.0, we will maintain security updates for the current major version.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :warning: Pre-release, limited support |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security vulnerability in obcache-go, please report it privately.

### How to Report

1. **DO NOT** create a public GitHub issue for security vulnerabilities
2. Report security issues via [GitHub Security Advisories](https://github.com/1mb-dev/obcache-go/security/advisories)
3. Include as much information as possible:
   - Description of the vulnerability
   - Steps to reproduce the issue
   - Potential impact
   - Any suggested fixes

### What to Expect

- **Initial Response**: We will acknowledge your report within 48 hours
- **Status Updates**: We will provide regular updates on our progress
- **Resolution Timeline**: We aim to resolve critical security issues within 7 days
- **Disclosure**: We will coordinate with you on public disclosure timing

### Security Best Practices

When using obcache-go:

1. **Keep Dependencies Updated**: Regularly update to the latest version
2. **Validate Inputs**: Always validate data before caching
3. **Secure Key Generation**: Use appropriate key generation for sensitive data
4. **Monitor Cache Access**: Implement appropriate access controls
5. **Resource Limits**: Set appropriate cache size limits

## Bug Bounty

At this time, we do not offer a formal bug bounty program. However, we greatly appreciate security researchers who responsibly disclose vulnerabilities and will publicly acknowledge their contributions (with permission).

## Security Tooling

This project uses several security tools:

- **govulncheck**: Automated vulnerability scanning for Go dependencies
- **gosec**: Static security analysis for Go code
- **golangci-lint**: Comprehensive linting including security checks
- **Dependabot**: Automated dependency updates

## Contact

For security-related questions or concerns:
- GitHub: @1mb-dev
- Security Issues: [GitHub Security Advisories](https://github.com/1mb-dev/obcache-go/security/advisories)

For general questions about obcache-go, please use GitHub Issues or Discussions.