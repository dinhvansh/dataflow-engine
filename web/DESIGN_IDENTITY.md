# DataFlow // Signalroom — visual identity v1

Scope: UI-only, stacked after PR #32. Does not change API, worker, SQL Server, ClickHouse or metadata.

## Why

The previous interface had the typical generic AI dashboard look: saturated blue gradients, pill badges, same rounded cards, equal-weight sections and unstructured whitespace. The Operations Center is a tool for **finding signal among pipeline events**, not a marketing landing page.

## Design language

- **Two distinct work modes**: Dark is graphite + volt for operations. Light is **white + blue** (#F7FAFF background, pure-white surfaces, navy text, royal-blue action accents). No mint/green surface tint; green is reserved for real success.
- **Signal rails**: a 3px leading bar for selected navigation, inspector panels, DAG nodes and cards. Green/red/yellow must retain their actual execution meanings; accent does **not** mean success.
- **Editorial / technical type**: strong readable sans for headings and data; small monospace for IDs, status, breadcrumbs and technical annotations. Avoid using monospace for lengthy body paragraphs.
- **Grid**: subtle 24px graph-paper background only on the DAG canvas. This is a system affordance, not decoration over every page.
- **Square-cut surfaces**: max 3–5px corner radius for operational panels; don't stack shadows/gradients.
- **Density**: align labels, numbers and status; compact tables and detail panels. Give data more visual weight than empty decoration.
- **Truth first**: no mocked production KPI; no invented node timing, success, throughput. User previews may use **clearly labeled QA synthetic fixtures**, never passed off as live.
- **Accessibility**: contrast-aware palette, keyboard focus visibility, non-color labels and reduced-motion support. Both themes preserve error/success legibility.

## Boundaries

Components remain local shadcn-style React/Tailwind primitives. No Ant Design and no CDN font requirement. Existing API contracts and on-click callbacks unchanged. PR can be reverted independently.

## Palette 03 · Graphite/Volt Dark + White/Blue Light

Color research: [PaletaColor Pro library](https://paletacolorpro.com/en/library). Reference palettes: [High Contrast Green](https://paletacolorpro.com/en/palette/high-contrast-green/004d40-00796b-ffffff-e0f2f1-1a1a1a), [Easy Reading](https://paletacolorpro.com/en/palette/easy-reading/2d2d2d-4a4a4a-ffffff-fff8dc-006400), [Accessible Alert](https://paletacolorpro.com/en/palette/accessible-alert/b71c1c-1b5e20-e65100-01579b-ffffff).

| Token | Dark | Light | Meaning |
|---|---|---|---|
| `--console-bg` | `#161D1B` | `#F7FAFF` | graphite/white-blue background |
| `--console-card` | `#202A26` | `#FFFFFF` | readable panels |
| `--console-accent` | `#C0F273` | `#245FC3` | active command, focus and brand |
| `--signal-teal` | `#75D5C5` | `#2B7CCD` | secondary information, not success |
| `--signal-status-success` | `#78DBAD` | `#167446` | actual observed successful state |
| `--signal-status-error` | `#F29890` | `#B42332` | actual observed failure |
| `--signal-status-warning` | `#EDCA83` | `#9B5816` | queued/running/unverified |

Light-theme user feedback (2026-10-17): mint/green page background felt muddy. Use clean white surfaces and restrained blue accents, while retaining the graphite/volt dark identity.\n\nKeep **brand accent** distinct from **execution success**. Accessible Alert shades are adjusted for context; validate text contrast for both themes and don't use color alone to convey status. Xyflow CSS reads the same semantic variables as cards, status rails and side navigation. No images copied from PaletaColor Pro.
