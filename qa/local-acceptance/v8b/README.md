# CORE-RUN-8B — Codex Local one-shot QA (current)

This replaces the **Core acceptance path** from the old v1.3 local QA mission; the v1.3 test catalog remains available separately for all other E/L/T features.

**No local QA has been executed merely by merging these files.** The owner may hand over the work to Codex Local once and receive an evidence package. Codex must report real outcomes, not claim PASS from prior hosted GitHub Actions.

## Scope (one run, isolated, finite)

The runner performs read-only Git/ownership/disk preflight, then **two negative/static safety contract stages before running builds or touching disposable volumes**: `STATIC-RESET-NEGATIVE` uses a stub Docker command to prove unfamiliar project volumes/networks are refused, and `STATIC-EVIDENCE-CONTRACT` validates the report schema with no Docker access. Go/Web builds and **fresh separately owned Docker QA volumes** follow:

1. **CORE-8B-E2E:** Playwright UI → Go → SQL Server 10k SELECT-only → PG/ClickHouse/dbt/DQ/publish; editable saved typed graph A→B→C and exact run/version, invalid fork, stale revision, physical rollback, zero-row replay, browser reload. Extended test: after launch, save graph revision 2; old version-1 run snapshot still unchanged and failed stale launches create no managed runs.
2. **CORE-6B-CLAIM:** real worker SIGKILL post PG claim, lease takeover/fence replay.
3. **CORE-6B-DQ:** 6 concurrent HTTP Core Runs (1 accepted, 5 conflicts), zero-row replay, true dbt DQ failure without publishing.
4. **CORE-6B-MID-SQL:** observe a real ClickHouse query while running, SIGKILL worker, rebuild/verify stage and atomic publish.
5. **CORE-6A-NETWORK:** API SIGKILL and ClickHouse link outage/explicit safe resume.

Each fault scenario requires an independent fresh metadata/raw fixture; therefore a full runner requires **explicit consent to delete only disposable volumes named `dataflow-localqa_*`**. `verify-reset-scope.sh` refuses any unknown container, volume, network or real ERP identifier. No `docker system prune`, no modification of existing non-QA services.

## Codex Local preparation

- Windows with WSL2 Ubuntu + Docker Desktop WSL integration, or Linux with Docker. Work on a fresh **local QA branch**, not `main`; run `git fetch`/status and reconcile divergence before touching files. Keep a clean tree. Do not reset/drop existing unrelated data.
- Go/Node/npm/Python3, pytest 8.4.2, Playwright 1.56.0 with Chromium; supply trusted CA certificates. `npm ci` and Go dependencies may need an allowed network. Do **not** disable certificate validation.
- At least **4 GiB free disk on QA workspace/Docker storage** according to `check-disk-capacity.sh` (this is DISK, not RAM). Observe CPU/RAM and configure disposable Docker limits. If resources insufficient report `BLOCKED-RESOURCE`, do not relax the guard.
- Generate ignored QA-only `.env.qa.local` via `bash deploy/local-qa/create-env.sh`; never supply real company passwords/server identities. All SQL Server DDL/DML lives strictly inside synthetic `DataFlowFixtureQA`; real ERP is SELECT-only and **not used** in this suite.
- Read the owned-volume list and explicitly approve scratch removal before starting.

```bash
python3 -m pip install 'pytest==8.4.2' 'playwright==1.56.0'
python3 -m playwright install chromium
bash deploy/local-qa/create-env.sh

DATAFLOW_QA_ONE_SHOT_OPT_IN=true \
DATAFLOW_QA_ALLOW_DISPOSABLE_RESET=I_APPROVE_OWNED_QA_VOLUMES \
bash deploy/local-qa/run-core8b-one-shot.sh
```

The script **stops at first FAIL** and leaves containers/volumes untouched for root-cause analysis; it does not silently reset the failed fixture. Codex may fix bugs on its own QA branch, add a regression and rerun the entire one-shot suite after preserving the prior failure evidence. Do NOT merge before exact-head CI PASS and owner review.

## Required evidence

- `qa-results/local-core8b-<UTC>/report.json`, `HANDOFF_TO_CHATGPT.md`, `matrix.tsv`, `git-*.txt`, and `logs/*.log`.
- QA Playwright artifacts in `qa-results/core-run5/`; fault artifacts in `qa-results/core-run6b*` and `qa-results/core-run6/`. Collect these **after secret scanning** and provide exact paths; do not upload raw .env, bearer, SQL credential or operational customer data.
- A report per case PASS/FAIL/BLOCKED/NOT RUN with real run IDs, row reconciliation, checkpoint generation, target physical UUID, duration, DB state, and known bugs. Never count `NOT RUN` as PASS.
- Use [TEST_MATRIX](TEST_MATRIX.md) and [CODEX_PROMPT](CODEX_PROMPT.md) for human verification and the single handoff prompt.

## Intentional gaps, not hidden tests

- CORE-RUN-7 **7 million rows**: user explicitly deferred it; `run-core-7m.sh` must not be invoked by this runner. No 16 GB RAM requirement is imposed by this suite.
- Owner-local signoff and live hardware/8GB workload: OPEN until Codex Local executes and supplies evidence.
- Production IdP/Vault/TLS, CDC source physical deletes, parallel/IF-ELSE and external resources: out of scope, **NO-GO**.

## One executable owner-local technical gate

The `run-core8b-one-shot.sh` command runs Go race/unit/vet/build, TypeScript build and mock Browser (reported separately), then a **fresh verified disposable QA fixture per live scenario**. The real Core-8B Playwright test now rejects unauthenticated graph catalog access with HTTP 401 in addition to saved-graph version/cursor/frozen snapshot checks. Every live PASS requires fresh structured JSON evidence; stale artifacts cannot be reused to claim success. After the live stages, the runner writes `02_TEST_MATRIX.csv` and calls the repository's existing `verify_core_final_evidence.py --report` gate. An empty/missing stage is NOT_RUN, not PASS. Full local execution remains PENDING until the owner authorizes resetting the **labeled disposable** QA volume set.

## Before handing off to Codex Local (reviewed 2026-10-19)

- The one-shot runner logs `STATIC-RESET-NEGATIVE` and `STATIC-EVIDENCE-CONTRACT` with independent PASS/FAIL results in `matrix.tsv` and `HANDOFF_TO_CHATGPT.md`. Neither stage launches/stops Docker workloads.
- **Explicit owner consent is still mandatory** before the script can reset even disposable QA volumes. Providing the Codex task prompt does **not** authorize deleting previously created containers or stored work. Codex must first show the owner the proposed disposable project reset scope; if approval is withheld, report BLOCKED.
- A green GitHub static contract test does not prove this runner has already executed on the owner's 8GB computer. Deferred 7m remains NOT RUN; production NO-GO.

## Real Canvas GUI editing gate (added after one-shot package review)

The canonical `qa/browser/test_core_live.py` E2E **also exercises the React Flow editor controls** after browser reload: load saved graph, add a deliberately invalid extra TRANSFORM, observe a server-rejected Validate, remove that transform, click the QUALITY node, change the DQ key, Validate then Save revision 2 in the GUI. It checks the PostgreSQL registry's changed DQ binding, immutable old Run revision 1, and a screenshot `operator-editor-revision.png`. This ensures green API-only graph tests cannot stand in for editing UX. It is part of the existing one-shot `CORE-8B-E2E` stage; no separate runner, fixture or hardware-heavy test is required.
