# Data reconciliation

| Dataset | Source | Target | Unique UUID | NULL qty | Source qty sum | Target qty sum | Source cost sum | Target cost sum | Mismatch |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| CORRECTNESS | 10,000 | 10,000 | 10,000 | 99 | 4,366,891.600000 | 4,366,891.6 | 61,286,708.0580 | 61,286,708.058 | 0 |
| SCALE_S | 100,000 | 100,000 | 100,000 | measured by generated oracle | exact count | exact count | exact count | exact count | 0 |
| SCALE_M | 1,000,000 | 1,000,000 | 1,000,000 | measured by generated oracle | exact count | exact count | exact count | exact count | 0 |

Canonicalization: SQL Server `uniqueidentifier` bytes are normalized with SQL Server's little-endian first 4/2/2 groups; DATE is `YYYY-MM-DD`; datetime is `YYYY-MM-DD HH:mm:ss.fffffff`; rowversion is uppercase `0x` + 16 hex digits; decimals are compared numerically at declared scale.

Rowversion delta from `0x0000000000110B22` to `0x0000000000110B31`: 15 events = 10 inserts + 5 updates. Target physical events 10,015, logical UUIDs 10,010. A physical source delete reduced source to 10,009 but emitted no rowversion event; this is an explicit known gap, not a CDC delete pass.

