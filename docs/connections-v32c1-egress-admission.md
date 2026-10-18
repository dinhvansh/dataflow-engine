# V3.2C-1 — endpoint admission boundary (OFFLINE, synthetic)

**Stage:** Test Connection safety foundation only. There is NO HTTP Test Connection endpoint, no database dial, no secret input, and no ability to configure destinations from the UI. Production NO-GO.

## Admission contract
- Create `connectionnet.Policy` **only** with trusted operator-provisioned exact hostname:port pairs, explicit CIDR ranges and resolver. No client input is allowed to extend those rules.
- Check strict lowercase DNS name / explicit port. Reject URL schemes, credentials, path, query, ports embedded in host, wildcard/suffix matching, localhost, invalid labels, empty/broad IPv4/IPv6 default routes, and disallowed address classes.
- Resolve the host using a supplied resolver, inspect **every** DNS record, and reject the entire admission when one address is unapproved, malformed, link-local, loopback, multicast, IPv4-mapped IPv6, metadata service IP, or outside the approved CIDR list. DNS failure and oversized responses fail closed.
- Return a stable sorted, deduplicated, immutable **pinned IP:port** plan and original hostname as TLS server name. The subsequent connector (NOT implemented in this PR) **must dial the returned pinned IP**, rather than re-resolving the DNS name. TLS certificate validation must use `ServerName()`; prohibit HTTP redirects and unsolicited alternate endpoints.
- Private RFC1918 corporate addresses are permissible **only** if an operator approves exact host, port and bounded private CIDR. This allows future on-prem Epicor tests without permitting arbitrary internal network probing.
- No DNS resolution in unit tests: injected fake resolver covers approved dual A, mixed public/private answer sets, metadata address, IPv6, unresolved name, untrusted host/port, invalid CIDR, canceled context and rebinding scenarios.

## Required later stages
V3.2C-2 must wire this plan into an isolated guarded **TLS/SQL Server READ-ONLY / ClickHouse scratch-only** dialing adapter, enforce context/byte/time/rate budgets and sandbox network egress outside Go code, then expose only an authenticated `TEST` endpoint with audit and sanitized errors. V3.2B remaining production IdP/Vault bootstrap, real tenant provisioning and credential lifecycle still block any activation. Never connect or mutate Epicor from CI.
