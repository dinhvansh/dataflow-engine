# DataFlow // Signalroom — visual identity v1

Scope: UI-only, stacked after PR #32. Does not change API, worker, SQL Server, ClickHouse or metadata.

## Why

The previous interface had the typical generic AI dashboard look: saturated blue gradients, pill badges, same rounded cards, equal-weight sections and unstructured whitespace. The Operations Center is a tool for **finding signal among pipeline events**, not a marketing landing page.

## Design language

- **Graphite + Volt**: dark ink-green graphite, warm near-white, one chartreuse signal accent. Light mode is a warm pale mineral background, not glaring white.
- **Signal rails**: a 3px leading bar for selected navigation, inspector panels, DAG nodes and cards. Green/red/yellow must retain their actual execution meanings; accent does **not** mean success.
- **Editorial / technical type**: strong readable sans for headings and data; small monospace for IDs, status, breadcrumbs and technical annotations. Avoid using monospace for lengthy body paragraphs.
- **Grid**: subtle 24px graph-paper background only on the DAG canvas. This is a system affordance, not decoration over every page.
- **Square-cut surfaces**: max 3–5px corner radius for operational panels; don't stack shadows/gradients.
- **Density**: align labels, numbers and status; compact tables and detail panels. Give data more visual weight than empty decoration.
- **Truth first**: no mocked production KPI; no invented node timing, success, throughput. User previews may use **clearly labeled QA synthetic fixtures**, never passed off as live.
- **Accessibility**: contrast-aware palette, keyboard focus visibility, non-color labels and reduced-motion support. Both themes preserve error/success legibility.

## Boundaries

Components remain local shadcn-style React/Tailwind primitives. No Ant Design and no CDN font requirement. Existing API contracts and on-click callbacks unchanged. PR can be reverted independently.
