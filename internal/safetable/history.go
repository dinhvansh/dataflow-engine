package safetable

import (
 "context"
 "errors"
 "fmt"
 "strconv"
 "strings"
)

// HistoryFingerprint is a best-effort integrity witness for all CDC rows at
// or below a checkpoint. This detects late/edited events at the next boundary;
// it is not a lock on a concurrently written ClickHouse delta table.
type HistoryFingerprint struct {
 Events uint64
 Digest string
}

// DeltaHistory recomputes a bounded-query-memory, order-independent witness
// over the FULL historical prefix, including payload and tombstone markers.
// This is O(total CDC history), intentionally TEST-only until an ingestion
// sequence or immutable manifest is introduced.
func (c Client) DeltaHistory(ctx context.Context, target, delta, key string, through uint64) (HistoryFingerprint, error) {
 var result HistoryFingerprint
 payload, err := c.ValidateDeltaSchema(ctx, target, delta, key)
 if err != nil { return result, err }
 cols := append(append([]string(nil), payload...), "_df_version", "_df_deleted")
 // Identifiers have been validated by ValidateDeltaSchema.
 q := fmt.Sprintf(
  "SELECT count(),toString(sum(toUInt128(cityHash64(tuple(%s))))) FROM scratch.%s WHERE _df_version<=%d FORMAT TabSeparatedRaw",
  strings.Join(cols, ","), delta, through,
 )
 raw, err := c.Query(ctx, q)
 if err != nil { return result, err }
 fields := strings.Split(raw, "\t")
 if len(fields) != 2 { return result, errors.New("invalid historical CDC fingerprint") }
 count, err := strconv.ParseUint(strings.TrimSpace(fields[0]), 10, 64)
 if err != nil { return result, err }
 digest := strings.TrimSpace(fields[1])
 if digest == "" { return result, errors.New("empty historical CDC digest") }
 result.Events, result.Digest = count, digest
 return result, nil
}
