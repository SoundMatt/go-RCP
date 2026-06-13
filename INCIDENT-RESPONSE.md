# Incident Response Plan

**Project:** go-RCP
**Standard:** IEC 62443-4-2 CR 6.2.1
**Owner:** Matt Jones <matt@jellybaby.com>

## 1. Scope

This plan covers security incidents affecting the go-RCP library and its downstream
integrations in automotive zonal control systems.

## 2. Incident Categories

| Severity | Description                                          | Response SLA |
|----------|------------------------------------------------------|--------------|
| Critical | Remote code execution, authentication bypass, data   | 24 hours     |
|          | corruption in safety-critical command path           |              |
| High     | Privilege escalation, zone misdirection, denial of   | 72 hours     |
|          | service on command transport layer                   |              |
| Medium   | Local information disclosure, non-critical DoS       | 14 days      |
| Low      | Configuration weaknesses, documentation gaps         | 90 days      |

## 3. Detection

Incidents may be detected via:
- Private vulnerability reports to `matt@jellybaby.com`
- GitHub Dependabot / OSV alerts (`gofusa vuln` in CI)
- User-reported operational anomalies
- Internal code review or `gofusa cyber` findings

## 4. Response Procedure

### 4.1 Triage (within SLA above)
1. Acknowledge receipt to the reporter.
2. Reproduce the issue in an isolated environment.
3. Assign a severity level using the table above.
4. Create a private tracking entry in `.fusa-problems.json` (`gofusa pr add`).

### 4.2 Containment
1. Determine whether a workaround can be documented immediately.
2. If a critical vulnerability is confirmed, prepare a patch branch.

### 4.3 Remediation
1. Develop the fix with tests and update `.fusa-reqs.json` if needed.
2. Open a private PR; review and merge.
3. Tag a patch release.

### 4.4 Disclosure
1. Publish a security advisory on GitHub once the patch is available.
2. Notify affected downstream users by email where known.
3. Update `SECURITY.md` with any policy changes.

## 5. Post-Incident Review

Within 7 days of resolution, document:
- Root cause
- Detection gap (why was it not caught earlier?)
- Process improvements

Record findings in `.fusa-problems.json` and close the tracking entry.
