# Go connector contract (design sketch)

```go
type TypedValue struct {
 Type string
 Bytes []byte
 Text string
 IsNull bool
}
type CursorKey struct { Values []TypedValue }
type PageRequest struct {
 LowerInclusive CursorKey
 UpperExclusive CursorKey
 After *CursorKey
 BatchSize int
}
type Batch struct {
 RunID string
 StreamID string
 Seq uint64
 Rows []Record
 LastKey CursorKey
 SchemaHash string
}
type SourceConnector interface {
 Discover(ctx context.Context, table TableRef) (TableSchema, error)
 Capabilities(ctx context.Context) (SourceCapabilities, error)
 SafeUpperBound(ctx context.Context, table TableRef) (TypedValue, error)
 ReadPage(ctx context.Context, table TableRef, req PageRequest) (Batch, error)
}
type DestinationConnector interface {
 Prepare(ctx context.Context, stream StreamSpec) error
 WriteBatch(ctx context.Context, batch Batch) (WriteReceipt, error)
 Verify(ctx context.Context, receipt WriteReceipt) error
 Publish(ctx context.Context, generation string) error
}
type StateStore interface {
 ClaimStream(ctx context.Context, streamID string) (Lease, error)
 LoadCheckpoint(ctx context.Context, streamID string) (Checkpoint, error)
 AckBatch(ctx context.Context, lease Lease, receipt WriteReceipt) error
 CommitWindow(ctx context.Context, lease Lease, next Checkpoint) error
}
```

Sketch only; surrounding types not defined yet. SQL Server is authoritative for UUID comparison ordering. Inject faulting fake connectors and verify retries/leases before production.
