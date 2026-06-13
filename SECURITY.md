# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| main    | :white_check_mark: |
| < v0.1  | :x:                |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately by emailing **matt@jellybaby.com** with the subject
line `[go-RCP SECURITY] <short description>`.

Include:
- A description of the vulnerability and its potential impact
- Steps to reproduce (proof of concept if available)
- Affected versions / configurations
- Any suggested mitigations

We will acknowledge receipt within **2 business days** and aim to provide a fix or
mitigation within **14 calendar days** for critical issues, or **90 days** for lower
severity findings.

## Security Requirements

go-RCP targets deployment in automotive safety-critical environments (ISO 26262 ASIL-B,
IEC 62443 SL-2). The security posture reflects these requirements:

- **Command integrity:** Zone and CommandID fields must be validated by the recipient
  to prevent misdirected command delivery.
- **Transport isolation:** The mock transport is process-local. Network transports
  (planned) will be constrained to declared network segments.
- **No remote code execution surface:** No dynamic plugin loading; all transports are
  compiled in.
- **Dependency minimisation:** The core `rcp` and `mock` packages have zero
  non-standard-library dependencies.
- **Watchdog support:** `CmdWatchdog` is a first-class command type to support
  hardware watchdog integration in zone controllers.
