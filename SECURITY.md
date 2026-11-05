# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities **privately** — do not open a public issue.

- **Preferred:** [GitHub private vulnerability reporting](https://github.com/blueshark0/Portico/security/advisories/new)
- Or email the maintainer via the address on the [maintainer profile](https://github.com/blueshark0)

Include a description of the issue, affected version/commit, and steps to reproduce if possible.

## Response

- We aim to acknowledge reports within **3 business days**.
- Critical issues (remote credential exposure, arbitrary config overwrite) are prioritized for a fix release as soon as possible; lower-severity issues are batched into regular releases.
- We will coordinate disclosure with the reporter and credit you (unless you prefer otherwise).

## Scope

In scope:

- The local relay (`127.0.0.1:18100`): auth checks, header forwarding, request/response handling
- Platform proxy config rewriting (`~/.claude`, `~/.codex`, `~/.gemini`)
- MCP gateway upstream connection handling (credential exfiltration via redirects, etc.)
- WebDAV config sync encryption (`internal/configsync`)
- Storage of provider API keys (`~/.portico/app.db`) and field-snapshot state files

Out of scope / accepted design:

- `InsecureSkipVerify` per-provider TLS skip is an **explicit user opt-in** for local intercepting proxies — it is not a vulnerability by itself
- The relay binds to loopback only; exposure beyond localhost requires a deliberate user action
- Threat model assumes the local user account is trusted (Portico manages credentials for that user's CLIs by design)
