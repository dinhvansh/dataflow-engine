# V3.2B-5D — optional direct TLS 1.3 listener, isolated TEST only

This PR provides the missing *transport* half of V3.2B-5A's direct-TLS-only JWT metadata GET route. It does NOT deploy a real IdP, issue credentials, enable connection execution, or make production readiness claims.

- Original Go `http.Server.ListenAndServe` behavior remains unchanged by default.
- When **both** `DATAFLOW_ENABLE_DIRECT_TLS_TEST=true` and `DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB=true`, the same server accepts HTTPS via `ListenAndServeTLS` (TLS >= 1.3). It uses externally operator-provisioned certificate and private key file paths. Key must be a private regular file (permissions no group/other bits), and pair must match and be within certificate validity dates.
- When `DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST=true`, startup fails without direct TLS; the previous plaintext listener cannot accidentally expose an enabled JWT metadata route. Incomplete/expired/mismatched cert/key also fail startup. `X-Forwarded-Proto` is not accepted as proof of TLS.
- Credential/private key contents are not stored in Git/PG, never returned by routes, and errors don't print raw cert material.
- Synthetic certificate/key tests cover default disabled, missing isolated-lab gate, valid local EC keypair, TLS 1.3 minimum, expiry, mismatched key and insecure key permissions. No production hostnames/certificates or ERP networks are contacted.

**Outstanding before live operation:** operator-approved HTTPS ingress, certificate rotation/renewal and monitoring, trusted IdP enrollment and JWKS rollover, workspace MANAGE bootstrap, Vault endpoint/policy/egress, durable recovery operator lifecycle, and security review. Passing local TLS tests is not a security audit, penetration test or public production approval.

Source Epicor remains SQL SELECT-only; no SQL Server, ClickHouse production or Vault credential mutation was introduced.
