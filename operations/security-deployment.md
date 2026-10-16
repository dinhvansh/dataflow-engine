# Security and deployment

Roles: viewer, operator, developer and admin. All endpoints must enforce RBAC and audit changes.

Epicor SQL Server is strictly SELECT-only, with limited connection count and bounded query time. ClickHouse ingestion and SQL job identities have separate least-privilege permissions. PostgreSQL runtime and migration identities are separate.

Use a secret manager with references in PostgreSQL, TLS and log redaction. Deploy Go API/workers, PostgreSQL and Web UI in containers, with volumes, backups and resource caps. Validate restore procedures before production.
