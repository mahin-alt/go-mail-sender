# 0002: No authentication for now

**Status:** accepted (revisit before any exposure beyond localhost)

## Context
The dashboard needs login in principle: the API can send email, expose recipient data, and the settings endpoints can change the SMTP server and credentials. A single-admin login (credentials from environment, signed short-lived token, middleware) was designed and offered.

## Decision
Skip authentication for now.

## Consequences
- **The API must not be reachable beyond localhost.** Anyone who can reach it can send mail, read recipient data and redirect outgoing email.
- `POST /settings/smtp/test` makes the server connect to any host/port it is told to; auth is what would make that safe.
- When added, the plan was: single admin first (roles later), `Authorization` header, `401` from a middleware on everything except `/healthz`, and Settings editing restricted to it.
- Activity Log entries currently cannot name a user.
