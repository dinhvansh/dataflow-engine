# 03 — Cursor, rowversion and consistency

> [RISK] Native binary support is easy; concurrent-write correctness is not.

## Epicor types
`SysRevID` is expected SQL Server timestamp/rowversion binary(8); verify each table. `SysRowID` is commonly uniqueidentifier; verify table PK. Preserve native types in SQL bindings. Hex is only serialized checkpoint form. SQL Server uniqueidentifier ordering differs from raw bytes ordering; never assume Go byte comparison reproduces SQL ORDER BY.

## Example typed checkpoint
```json
{
 "stream":"Erp.PartTran",
 "mode":"rowversion_incremental",
 "cursor_fields":[
  {"name":"SysRevID","sql_type":"rowversion","value":"0x000000000001A2B3"},
  {"name":"SysRowID","sql_type":"uniqueidentifier","value":"00000000-0000-0000-0000-000000000001"}
 ],
 "upper_exclusive":"0x000000000001A400",
 "checkpoint_version":12
}
```

## Safe upper bound and transaction semantics
`@@DBTS` is not a commit watermark. `MIN_ACTIVE_ROWVERSION()` provides a boundary relevant to active rowversions. Choose an upper-exclusive fence at the start of a run and scan only below it, *subject to validated isolation and source semantics*. A row can be updated and move beyond the fence during pagination; strict consistency needs controlled snapshot strategy or explicit verified reconciliation. Do not promise capture of every intermediate event.

## Proposed window protocol [VERIFY]
1. Acquire fenced per-stream lease; load persisted lower-inclusive cursor.
2. Obtain upper-exclusive boundary `MIN_ACTIVE_ROWVERSION()` before reading.
3. Keyset page `SysRevID >= lower AND SysRevID < upper`, ORDER BY `SysRevID, SysRowID`, with SQL typed parameters.
4. Stage batches with event key (stream, PK, SysRevID) and immutable sequence.
5. Persist writer ACKs; contiguous frontier only.
6. Commit next lower-inclusive = upper *only if window completely processed and consistency contract satisfied*, guarded by generation CAS and lease fencing.
7. Crash ⇒ replay from durable checkpoint; destination must dedup.

```sql
SELECT TOP (@batch) *
FROM Erp.PartTran
WHERE SysRevID >= @lower_binary8 AND SysRevID < @upper_binary8
  AND (SysRevID > @page_rev OR
      (SysRevID = @page_rev AND SysRowID > @page_rowid))
ORDER BY SysRevID, SysRowID;
```
Omit page continuation for first page; verify actual plan and source isolation. Cursor fields are typed ordered tuples with explicit null/collation handling. Checkpoint must never denote incomplete or unverified ranges.

## Verification
Test long uncommitted TX, out-of-order commits, rollback, row movement mid-scan, restart, duplicate/tied cursor, schema drift and SQL UUID ordering. Rowversion cannot detect physical DELETE; see [change detection](04-change-detection.md).
