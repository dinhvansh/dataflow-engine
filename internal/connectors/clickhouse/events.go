package clickhouse

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "strings"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/dinhvansh/dataflow-engine/internal/engine"
)

// EventWriter stores a durable append-only change log.
// Identical (stream, PK, source rowversion) can be physically repeated after
// an interrupted run; consumers MUST use the documented version-aware view.
type EventWriter struct {
 Destination *Writer
 StreamID string
 RunID string
}
func(w *EventWriter) Write(ctx context.Context,b engine.Batch)error{
 if w.Destination==nil {return errors.New("missing ClickHouse destination")}
 out:=engine.Batch{Sequence:b.Sequence,Rows:make([]map[string]any,0,len(b.Rows))}
 for _,record:=range b.Rows{
  key,ok:=record["SysRowID"].(string)
  if !ok||strings.TrimSpace(key)==""{return errors.New("missing source SysRowID")}
  rev,ok:=record["SysRevID"].(string)
  if !ok{return errors.New("missing source SysRevID")}
  parsed,err:=cursor.ParseRowVersion(rev)
  if err!=nil{return fmt.Errorf("invalid SysRevID: %w",err)}
  payload,err:=json.Marshal(record)
  if err!=nil{return fmt.Errorf("encode payload: %w",err)}
  out.Rows=append(out.Rows,map[string]any{
   "_stream_id":w.StreamID,
   "_pk":strings.ToLower(key),
   "_sysrevid":parsed.String(),
   "_payload":string(payload),
   "_run_id":w.RunID,
   "_batch_seq":b.Sequence,
  })
 }
 return w.Destination.Write(ctx,out)
}
