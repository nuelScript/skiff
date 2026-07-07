# Security Policy

Skiff builds and runs code and manages servers, so we take security seriously
and appreciate responsible disclosure.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
pull requests, or discussions.**

Instead, report privately through either:

- **GitHub Security Advisories** — on the repository, go to the **Security** tab
  → **Report a vulnerability** (preferred), or
- **Email** — **igwee3333@gmail.com** with the subject `SECURITY: <short
  description>`.

Please include enough detail to reproduce:

- the affected component (CLI, control panel, router/proxy, builder, …),
- a description of the issue and its impact,
- steps to reproduce or a proof of concept, and
- any suggested remediation, if you have one.

## What to expect

- We aim to acknowledge a report within **72 hours**.
- We'll investigate, keep you updated on progress, and let you know when a fix
  is released.
- We'll credit you in the release notes / advisory once the issue is resolved,
  unless you'd prefer to remain anonymous.

Please give us a reasonable amount of time to release a fix before any public
disclosure.

## Supported versions

Skiff is pre-1.0 and moves quickly. Security fixes land on `main` and in the
**latest release**; please make sure you're on the latest version before
reporting.

## Scope

In scope: the Skiff CLI, control panel, edge router/proxy, builders, and the
deploy pipeline in this repository.

Out of scope: vulnerabilities in third-party dependencies (report those
upstream), and issues that require an already-compromised host or physical
access to the box Skiff runs on.
