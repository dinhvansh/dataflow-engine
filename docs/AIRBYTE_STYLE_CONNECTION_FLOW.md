# Connection flow inspired by Airbyte

Reference inspected: `airbytehq/airbyte-platform` at `365c6ea` (27 October 2026), especially `DefineSource`, `DefineDestination`, `SyncCatalogTable`, and `ConnectionConfiguration`. This document describes DataFlow's own QA implementation and does not copy Airbyte components.

The primary route is **Sources → Destinations → Connections → Streams → Sync**. Sources and Destinations show saved profiles and their Test action. Connections shows real saved connections and leads to one creation flow. Discovery lists tables without enabling them. Selecting a table loads only its columns; search and All / Enabled / Disabled filters keep larger schemas usable. The final step reviews modes, targets, and schedule before saving.

UI references in Airbyte's source: `DefineSource` / `DefineDestination` use a choice between an existing connector and a new one, a search box, and a list of saved connectors. `SyncCatalogTable` uses a searchable, filterable table with expandable stream rows and field selection. `ConnectionConfiguration` separates stream selection from connection settings. DataFlow now follows that layout with its own light QA workspace, inline stream fields, a final review, and connection status. The implementation remains DataFlow code; no Airbyte source component or asset is copied.

```mermaid
sequenceDiagram
    actor User
    participant UI as DataFlow console
    participant API as Go API
    participant PG as PostgreSQL
    participant SQL as QA SQL Server
    participant CH as QA ClickHouse
    participant Worker as Sync worker

    User->>UI: Choose or create Source
    UI->>API: Save and Test SQL Server profile
    API->>SQL: Read-only connection check
    API-->>UI: Verified profile
    User->>UI: Choose or create Destination
    UI->>API: Save and Test ClickHouse profile
    API->>CH: Scratch permission check
    API-->>UI: Verified profile
    User->>UI: Create Connection
    UI->>API: Discover tables (schema)
    API->>SQL: Read metadata
    API-->>UI: Available tables, none selected
    User->>UI: Select streams and sync modes
    loop Each selected stream
        UI->>API: Load columns for selected table
        API->>SQL: Read column metadata
        API-->>UI: Columns, key and cursor candidates
    end
    User->>UI: Review destination, schedule and save
    UI->>API: Create connection and bind streams
    API->>PG: Persist connection and stream settings
    API-->>UI: Saved stream IDs
    opt User runs a stream
        User->>UI: Run
        UI->>API: Enqueue sync
        API->>PG: Save job
        Worker->>SQL: SELECT from verified source
        Worker->>CH: Stage and verify QA target
        Worker->>PG: Commit run result and checkpoint
        UI->>API: Poll run status
        API-->>UI: Result and row count
    end
```

This remains limited to the isolated SQL Server → ClickHouse QA path. The UI must not imply that planned connectors or production ERP operations are available.

Verification is valid for 30 minutes. When it expires, the connection shows Retest profiles; a successful Test refreshes verification without changing the profile revision, so existing bindings stay valid. Editing a profile or failing a Test invalidates its old bindings. Re-testing after that does not revive them; create a new connection for the affected streams.
