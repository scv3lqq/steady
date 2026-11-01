# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub: open the repository's Security tab and choose "Report a vulnerability" (private vulnerability reporting). Do not open a public issue for a security problem.

## Design notes

- Steady is a single-user app with no authentication. Anyone who can reach the port can read and change all data, so the default listen address is `127.0.0.1` and it should stay on a loopback address.
- The server checks the `Host` header against an allowlist (DNS rebinding) and rejects cross-origin browser requests that change data.
- Every response carries a Content-Security-Policy and other hardening headers.
- Release binaries come with a build provenance attestation, see the Deploy section of the README.

See the Security section of the README for details.
