# DataFlow Console (shadcn/ui style)

Vite + React + TypeScript + Tailwind + Radix Slot + shadcn component conventions (`components.json`, `src/components/ui`).

## Local developer setup
```bash
cd web
npm install
npm run dev
```
Open http://localhost:5173. Vite proxies `/api` and `/healthz` to Go on localhost:8080.

The UI has five navigation areas: Overview (live backend health + last-session run), Sync Jobs (functional POST to experimental Go bootstrap), Run Inspector (functional status GET), Connections and Pipelines (explicitly disabled pending APIs).

**Warning:** Bootstrapped Go endpoint is synchronous, writes APPEND, and requires a precreated disposable ClickHouse table. No persistent browser token storage. Keep the browser session open while running. For internal development only.

## Production gaps
Proper shadcn CLI component generation/lockfile, UI tests, authenticated sessions and roles, async runs, persisted job CRUD, schema discovery, incremental rowversion, scheduler and DAG, ClickHouse staging/idempotency. Do not claim this is a finished management UI.
