# Connector Hub 3-tab & QA Wizard V2 (PR QA handoff)

## Scope
Frontend-only changes based on GitHub `main` at `c56b42b6cd97b865956ff166ce03544ee282c355`.
The locally reported commits `5649357` and `1462cf5` and branch `feat/console-ux-redesign-20261024` were **not published on GitHub** when this branch was created.
**Do not merge until comparing/cherry-picking with that local branch**; avoid overwriting more recent UI.

## Updated UI
- `ConnectorHub.tsx`: uses 3 mounted tab panels (Connections / Sources / Destinations), defaults Sources with SourceExplorer, moves route preview/roadmap under Connections, retains source/destination catalogs, configured connections, draft registry.
- `RuntimeConnectionManager.tsx`: exposes real loaded QA profiles to wizard; original Add, encrypted Save, real Test and Bind remain intact, one instance above tabs; hides the obsolete embedded setup wizards in the new Hub to avoid duplication.
- `ConnectionWizardV2.tsx`: 4-step flow, verified profile checks, SELECT-only discovery, up to 10 streams, rowversion-mode gating, column-selection popup, review errors, QA batch create, optional schedule per stream, run/status polling.
- `ConnectionWizardV2.css`, `ConnectorHubShellV2.tsx`, `ConnectorHubShellV2.css`: semantic `--console-*` styles and slot layout.

## Backend contract caveats
- Existing raw batch endpoint chooses `scratch.dfqa_<alias>_<stream-id>`, **not** the exact `dfqa_<schema>_<table>` without suffix.
- Incremental currently supports SQL Server **native rowversion/timestamp** only. Datetime is displayed disabled with an explicit API limitation.
- Managed streams API does **not** list live connection status. UI shows loaded managed streams and honestly labels online state as unavailable.
- Backend enforces real verified profile freshness, SELECT-only grants, unique key, SNAPSHOT isolation, scratch endpoint allowlists; UI cannot prove those without disposable QA integration.

## Verification gates
- [ ] `cd web && npm ci && npm run build` PASS on PR CI.
- [ ] No forbidden Tailwind raw colors / `window.confirm()` in modified files.
- [ ] Browser: header Connector Hub; Sources selected on first open; SourceExplorer visible immediately.
- [ ] Role operator: no Live QA panel and no privileged wizard; admin: exactly one full manager above tabs.
- [ ] Connections tab: wizard, managed streams listing, configured, draft, route preview, roadmap.
- [ ] Wizard: profile draft disabled, discovery gating/reload, 10-table max, required columns, modal 3 columns, error review blocks submit.
- [ ] Verify no console errors and capture browser screenshots of success and blocked/error states.
- [ ] QA/lab only, **never SRVERPTEST / ERPDBTEST**.
- [ ] Compare to local `1462cf5` / `5649357` before any merge.

No backend/API changes. This PR is intended for CI and owner local QA, not production rollout.