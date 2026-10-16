# Isolated local QA stack

Run `./create-env.ps1`, `./up.ps1`, then the functional/fault/benchmark scripts.
On Linux the matching `.sh` entrypoints execute the same checks. The stack is
owned by Compose project `dataflow-localqa`, binds only loopback ports, and all
mutating fixture commands verify QA labels and `DataFlowFixtureQA` first.

`down` preserves named volumes. Never use these scripts with an Epicor DSN.

Preflight requires at least 4 GiB free on the workspace and Docker filesystems
before creating the stack. Override `DATAFLOW_QA_MIN_FREE_BYTES` only for a
smaller disposable fixture run; the delta benchmark enforces the same guard.

