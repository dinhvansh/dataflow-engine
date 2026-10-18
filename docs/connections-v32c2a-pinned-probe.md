# V3.2C-2A — bounded TCP transport probe (synthetic only)

**Status:** Internal transport reachability primitive, NOT a real Test Connection. No HTTP endpoint, Vault credential setup, driver handshake, SQL source reads or ClickHouse writes.

- Receives a `connectionnet.Plan` containing addresses admitted under the operator-pinned V3.2C-1 hostname/port/CIDR + DNS policy. The original DNS name is never resolved again during this probe.
- Attempts up to four already-pinned TCP IP:port addresses, with a total 3-second context timeout. A trusted server-side dialer must honor context cancellation; the address never comes from an HTTP payload.
- Closes a successfully opened connection; returns only `reachable` and bounded `attempts`. Driver/transport errors are sanitized and not passed to HTTP or audit logs.
- Synthetic CI injects a `net.Pipe`/mock dialer instead of opening a socket. Tests prove DNS rebinding after admission cannot redirect the probe, canceled requests and unsafe plans fail closed, retries never exceed four and raw errors are redacted.

**Before activating any TEST action:** real OIDC `TEST` RBAC + actor/workspace audit, approved network egress/SSRF policy at infrastructure level, verified pinned TLS certificate/server name, SQL Server read-only role, ClickHouse scratch-only role, driver-specific handshake, rate limits, UI status sanitization and documented credential/connection version lifecycle. Successful TCP transport reachability is NOT equivalent to valid DB credentials or query privileges.

**Epicor remains SELECT-only and was not contacted.**