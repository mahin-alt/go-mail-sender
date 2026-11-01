# 0011: Who should be allowed to change which setting (FUTURE, not done)

**Date:** 2026-11-01 · **Status:** proposed, parked on purpose · **Remind the user** when they plan login, a production deploy, or anything reachable beyond localhost.

## Context
Phase 4 shipped a Settings page where **every** editable setting can be changed by anyone who can reach the API (there is no login, decision 0002). The user chose to keep it that way for now and to be reminded about this later.

## The senior-backend view, in three tiers
| Tier | Settings | Why |
|---|---|---|
| 1. Any staff member | `DOMAIN_CHECK_ENABLED`, `MAX_JOB_RETRIES` | Harmless. Lower the retries cap from 20 to about 10. |
| 2. Admin only, after login | `SMTP_HOST/PORT/TLS_MODE/USERNAME/PASSWORD`, `MAIL_FROM` | Whoever can change these can redirect **every email** to their own server and read all recipients and messages. `MAIL_FROM` affects deliverability and spoofing. `SMTP_TLS_MODE=none` sends the password unencrypted; allow it only for localhost or warn loudly. |
| 3. Operations only (keep in `.env`) | `WORKER_COUNT`, `SMTP_POOL_SIZE`, `JOB_POLL_INTERVAL`, `HTTP_*`, `DATABASE_URL`, `CORS_ALLOWED_ORIGINS`, `SETTINGS_ENCRYPTION_KEY` | Mis-set values hurt (provider rate limits, pool smaller than workers, database hammering) and they need a restart the page cannot perform. |

## To do when this is picked up
1. **Kill switch:** an environment flag such as `SETTINGS_EDITABLE=false` that makes the whole Settings API read-only (cheap, valuable for any machine reachable beyond localhost).
2. **Validation:** require `SMTP_POOL_SIZE >= WORKER_COUNT` (no such check exists today); lower the `MAX_JOB_RETRIES` cap.
3. **Move to read-only on the page:** the three "Server limits" settings (they currently are editable with a "needs a restart" notice).
4. **Audit trail:** record who changed which setting, when, old to new (today the log has only the names that changed). Ties into the Activity Log feature.
5. **Login and an admin role** (decision 0002 is the blocker); then Tier 2 requires admin.
6. **Business-level knobs instead of infrastructure ones:** for example "send at most N emails per minute" rather than worker counts.

Tracked as "proposed" in [ROADMAP.md](../ROADMAP.md).
