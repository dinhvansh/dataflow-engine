// Package postgres implements durable, fenced, monotone cursor checkpoints.
// Do not call Commit until every batch in the SQL Server snapshot window has
// been durably written, verified and the source transaction has succeeded.
package postgres

import(
 "context"
 "errors"
 "fmt"
 "time"

 "github.com/dinhvansh/dataflow-engine/internal/cursor"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseHeld=errors.New("stream lease is already held")
var ErrLeaseLost=errors.New("stream lease expired or stolen")
var ErrRegression=errors.New("checkpoint cannot move backward")

type Store struct{Pool *pgxpool.Pool}
type Checkpoint struct{
 Lower cursor.RowVersion
 Generation int64
 FenceToken int64
}
type Lease struct{
 StreamID string
 OwnerID string
 FenceToken int64
 Lower cursor.RowVersion
 Generation int64
}

// Load reads the last fully committed lower-inclusive boundary.
func(s Store)Load(ctx context.Context,streamID string)(Checkpoint,error){
 var c Checkpoint;var raw []byte
 err:=s.Pool.QueryRow(ctx,`SELECT lower_cursor,generation,fence_token FROM sync_checkpoints WHERE stream_id=$1::uuid`,streamID).Scan(&raw,&c.Generation,&c.FenceToken)
 if err!=nil{return c,err}
 c.Lower,err=cursor.FromBytes(raw)
 return c,err
}

// Claim uses PostgreSQL time and an atomic row update to grant a fenced lease.
// Only one active owner may advance a stream checkpoint.
func(s Store)Claim(ctx context.Context,streamID,ownerID string,ttl time.Duration)(Lease,error){
 var lease Lease;lease.StreamID=streamID;lease.OwnerID=ownerID
 if ttl<5*time.Second||ttl>24*time.Hour{return lease,errors.New("lease TTL must be 5 seconds to 24 hours")}
 secs:=ttl.Seconds()
 var raw []byte
 err:=s.Pool.QueryRow(ctx,`
 UPDATE sync_checkpoints
 SET fence_token=fence_token+1,lease_owner=$2::uuid,
     lease_expires_at=clock_timestamp()+($3::double precision * INTERVAL '1 second'),updated_at=now()
 WHERE stream_id=$1::uuid AND
  (lease_owner IS NULL OR lease_expires_at<clock_timestamp())
 RETURNING lower_cursor,generation,fence_token`,
  streamID,ownerID,secs).Scan(&raw,&lease.Generation,&lease.FenceToken)
 if errors.Is(err,pgx.ErrNoRows){return lease,ErrLeaseHeld}
 if err!=nil{return lease,err}
 lease.Lower,err=cursor.FromBytes(raw)
 return lease,err
}
func(s Store)Renew(ctx context.Context,lease Lease,ttl time.Duration)error{
 if ttl<5*time.Second||ttl>24*time.Hour{return errors.New("lease TTL out of bounds")}
 tag,err:=s.Pool.Exec(ctx,`
 UPDATE sync_checkpoints SET lease_expires_at=clock_timestamp()+($4::double precision * INTERVAL '1 second')
 WHERE stream_id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3
 AND lease_expires_at>clock_timestamp()`,lease.StreamID,lease.OwnerID,lease.FenceToken,ttl.Seconds())
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return ErrLeaseLost}
 return nil
}
// Commit advances a completed window and releases the lease. No automatic
// Commit is performed by Reader/Writer. The caller owns completion validation.
func(s Store)Commit(ctx context.Context,lease Lease,next cursor.RowVersion)error{
 if next.Compare(lease.Lower)<0{return ErrRegression}
 tag,err:=s.Pool.Exec(ctx,`
 UPDATE sync_checkpoints SET lower_cursor=$5,generation=generation+1,
  lease_owner=NULL,lease_expires_at=NULL,updated_at=now()
 WHERE stream_id=$1::uuid AND lease_owner=$2::uuid
  AND fence_token=$3 AND generation=$4
  AND lease_expires_at>clock_timestamp() AND lower_cursor<=$5`,
  lease.StreamID,lease.OwnerID,lease.FenceToken,lease.Generation,next.Bytes())
 if err!=nil{return fmt.Errorf("checkpoint commit: %w",err)}
 if tag.RowsAffected()!=1{return ErrLeaseLost}
 return nil
}
func(s Store)Release(ctx context.Context,lease Lease)error{
 tag,err:=s.Pool.Exec(ctx,`
 UPDATE sync_checkpoints SET lease_owner=NULL,lease_expires_at=NULL
 WHERE stream_id=$1::uuid AND lease_owner=$2::uuid AND fence_token=$3`,
 lease.StreamID,lease.OwnerID,lease.FenceToken)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return ErrLeaseLost}
 return nil
}
