# DataFlow v1.0 — Data Lineage and Impact Analysis (developer preview)

## How to use
1. Create and save a Visual model `staging` using source `scratch.invoices`, then Build View (dbt) while in TEST mode.
2. In Transform Studio click Models, choose `staging` as the source for another model `revenue`. DataFlow sets source table `scratch.dfv_staging`.
3. Save `revenue`; a dependency edge `staging → revenue` is discovered automatically from the saved source definition.
4. Open **Data Lineage** from the main navigation. Click a model card to see its direct parents, children, all downstream impacted models, and latest dbt run status.
5. If `staging` has not been built successfully with its current SQL, Build View of `revenue` is refused until `staging` is rebuilt.
6. Saving a model that creates a dependency cycle is refused before PostgreSQL writes it. Concurrent saves are serialized by an advisory transaction lock.

## Safety contract
- Metadata lineage is derived **only from the first visual source** `scratch.dfv_<model_name>`. Arbitrary SQL editor dependencies and multi-source joins are marked untracked.
- Lineage **does not execute downstream models** or automatically rebuild them; the operator must build upstream first.
- Build View checks every ancestor model's most recent dbt run was successful and its SQL equals current saved compiled SQL. Missing, cyclic or stale upstream blocks queueing.
- This is **scratch View-only** dbt in TEST mode. Table/Incremental materialization, Mart/ODS, and production ERP writes remain prohibited.
- The UI shows the first 100 nodes for rendering efficiency, while the backend analyzes up to 500 saved models.
- Metadata links are not full SQL query parse lineage and cannot capture dependencies hidden in raw SQL, subqueries, data federation, or runtime external tables.
- The saved-name lineage prefix `dfv_` is reserved for generated dbt View names.
- The transaction lock protects concurrent saves/queues from cycle and stale version races; it is not a production multi-user RBAC scheme.
- CI tests deterministic graph ordering, fan-out impact, cycle handling, save rollback, absent upstream and stale SQL gate with live PostgreSQL.
- No live Epicor TEST full E/L/T integration or visual browser E2E is claimed.

## Example graph

`scratch.invoices → staging → revenue → sales_dashboard`

Change `staging` and Data Lineage highlights `revenue` and `sales_dashboard` as potentially impacted. Build buttons still require manual operation and up-to-date upstream View success.

Next: explicit multi-input dependencies, dbt `ref()` for same-project dependency build, column-level lineage, model-specific tests, and safe staged Table materialization.
