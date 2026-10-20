package clickhouse

import (
 "context"
 "errors"
 "regexp"
)

// QA raw destinations use the same versioned-event layout as core raw data.
// Only names with dfqa_ prefix are created by the isolated stream worker;
// no existing arbitrary or production table may be dropped/rewritten here.
var qaRawName=regexp.MustCompile(`^dfqa_[a-z][a-z0-9_]{0,29}_[0-9a-f]{12}$`)

func ValidQARawTarget(database,table string)bool{
 return database=="scratch"&&qaRawName.MatchString(table)
}

func(w *Writer)EnsureQARawTarget(ctx context.Context)error{
 if !ValidQARawTarget(w.Database,w.Table){return errors.New("not an isolated owned QA destination")}
 // Hardcoded DDL layout, no user-defined expression/table grammar.
 // IF NOT EXISTS supports retry after a crashed worker; never drop a table.
 return w.exec(ctx,"CREATE TABLE IF NOT EXISTS `scratch`.`"+w.Table+"` "+
  "(_stream_id String,_pk String,_sysrevid String,_payload String,"+
  "_run_id String,_batch_seq UInt64,_ingested_at DateTime64(3,'UTC') DEFAULT now64(3)) "+
  "ENGINE=MergeTree ORDER BY (_stream_id,_pk,_sysrevid)")
}
