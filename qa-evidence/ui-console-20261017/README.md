# DataFlow Engine — UI Console screenshots

Captured from the running synthetic QA Web Console on 2026-10-17. The API token fields were left empty and no credentials, production data, browser traces, or logs are included.

## Overview

![Overview](01-overview.png)

## Sync Jobs

![Sync Jobs](02-sync-jobs.png)

## Connections / Source Explorer

![Connections](03-connections.png)

## Transform Studio

![Transform Studio](04-transform-studio.png)

## Data Lineage

![Data Lineage](05-data-lineage.png)

## Incremental CDC

![Incremental CDC](06-incremental-cdc.png)

## Evidence and freshness

These six PNGs were captured on **17 October 2026** from the synthetic QA console (not live production). They were reviewed together in [PR #25](https://github.com/dinhvansh/dataflow-engine/pull/25); the API token inputs appear empty, and PNG-format/metadata inspection found no secret-bearing text chunks. Sample schema/table names and scratch fixtures are visible; no customer records or production connection strings appear in the reviewed previews.

**This is a historical UI snapshot.** Later UI and backend foundation PRs may have changed the look and capabilities. Screenshots do **not** prove live Epicor E2E, credential safety in every runtime state, Data Quality results or production readiness. Functional and E2E acceptance remain documented separately in the [canonical handoff](../../docs/PROJECT_HANDOFF_LATEST.md) and [master roadmap](../../docs/MASTER_ROADMAP_AND_STATUS.md).
