# Security Policy

## Supported Versions

Security fixes should target `main` unless the maintainers document release branches in the future.

## Reporting a Vulnerability

Do not open a public issue for a suspected vulnerability.

Report privately through GitHub's private vulnerability reporting if enabled for the repository. If it is not enabled, contact the maintainers using the private security contact listed by the adopting project.

Include:

- A concise description of the issue.
- Affected files, commands, or workflows.
- Reproduction steps or a proof of concept when safe to share.
- Potential impact and suggested mitigation.

## Security Baseline

This project keeps these controls enabled:

- `gosec`, `govulncheck`, `go vet`, and race-enabled tests in CI.
- Pre-commit hooks for file hygiene, formatting, vetting, imports, and linting.
- Secret scanning and branch protection in the hosting platform.
- Least-privilege tokens for CI and automation.
- Human review for dependency additions, workflow permission changes, and agent configuration changes.

## Agent Safety

Coding agents must not:

- Print or commit secrets, access tokens, credentials, cookies, or private keys.
- Exfiltrate repository data to unapproved external services.
- Run destructive commands without explicit human approval.
- Weaken security checks to make a task pass.
- Treat instruction files as an enforcement layer; use CI, hooks, and platform controls for enforcement.
