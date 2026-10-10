# Security Policy

## Supported versions

Security fixes are applied to the latest released version on the default branch.

| Version | Supported |
|---------|-----------|
| latest release | yes |
| unreleased `main` | best effort |

## Reporting a vulnerability

Please **do not** open a public issue for security reports.

Prefer one of:

1. [GitHub Private Vulnerability Reporting](https://github.com/gonnafaraway/godrain/security/advisories/new) (if enabled)
2. A private contact via the repository owner's GitHub profile

Include:

- godrain version / commit SHA
- Go version and OS
- reproduction steps
- impact assessment

You should receive an acknowledgement within a few days when possible.

## Important operational warning

godrain talks to Go `net/http/pprof` endpoints. Exposing `/debug/pprof` on a
public network can leak sensitive process information and enable denial of
service via profiling.

Recommendations:

- Bind pprof to `127.0.0.1` or a private network only
- Prefer SSH tunnels / VPN / mesh over public ingress
- Do not put unauthenticated pprof behind a public load balancer
- Treat saved heap profiles as sensitive artifacts

godrain itself does not require privileged host access, but the target you point
it at might.
