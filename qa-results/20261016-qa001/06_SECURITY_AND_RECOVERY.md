# Security and recovery

- SQL login `qa_erp_reader`: SELECT succeeds, UPDATE returns SQL Server error 229.
- Wrong API bearer: 401. Wrong destination database `production`: 403.
- ClickHouse outage: run returned 502; published target stayed at 10,000.
- PostgreSQL outage: run returned 500 before run creation; published target stayed at 10,000; restart healthy.
- Managed queue test verified lease renewal, stale fencing, three bounded claims and atomic checkpoint.
- Auto Build recreated the API server between A and B and completed without duplicate children.
- Cleanup scripts resolve exact container names and require both QA ownership labels.
- No write connection to Epicor TEST/production was configured. All source INSERT/UPDATE/DELETE occurred only in the labeled SQL Server container and `DataFlowFixtureQA`.

