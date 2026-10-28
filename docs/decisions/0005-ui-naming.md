# 0005: UI naming: "Bulk Emails", not "Campaigns"

**Status:** accepted

## Context
"Campaign" is the marketing term for one send-out, but this product sends operational emails (orders, merchant notices) and lacks marketing features such as scheduling or tracking.

## Decision
User-facing names: **Bulk Emails** (with New Bulk Email and All Bulk Emails), **Verify Emails**, **Single Email**, **Templates**, plus Overview, Failures, Recipients, Activity Log and Settings.
The API and database keep their technical names (`batches`, `email_jobs`).

## Consequences
- UI text and docs say "bulk email"; code, routes and tables say "batch". The glossary in [PROJECT.md](../PROJECT.md) maps one to the other.
- "Campaign" would have promised features that do not exist.
