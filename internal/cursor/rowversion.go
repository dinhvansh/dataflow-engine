// Package cursor defines typed SQL Server rowversion windows and batch ACK frontiers.
package cursor

import (
 "bytes"
 "encoding/hex"
 "errors"
 "fmt"
 "strings"
 "sync"
)

type RowVersion [8]byte

func ParseRowVersion(s string) (RowVersion, error) {
 var out RowVersion
 s = strings.TrimPrefix(strings.TrimPrefix(s,"0x"),"0X")
 if len(s)!=16 { return out, fmt.Errorf("rowversion requires 16 hexadecimal digits, got %d",len(s)) }
 raw,err:=hex.DecodeString(s); if err!=nil{return out,fmt.Errorf("invalid rowversion: %w",err)}
 copy(out[:],raw)
 return out,nil
}
func FromBytes(v []byte) (RowVersion,error) {
 var out RowVersion
 if len(v)!=8 {return out,fmt.Errorf("rowversion requires 8 bytes, got %d",len(v))}
 copy(out[:],v);return out,nil
}
func (v RowVersion) String() string {return "0x"+strings.ToUpper(hex.EncodeToString(v[:]))}
func (v RowVersion) Bytes() []byte {return append([]byte(nil),v[:]...)}
func (v RowVersion) Compare(other RowVersion)int{return bytes.Compare(v[:],other[:])}

type Window struct { LowerInclusive,UpperExclusive RowVersion }
func (w Window) Validate()error{
 if w.LowerInclusive.Compare(w.UpperExclusive)>0{return errors.New("lower cursor exceeds upper bound")}
 return nil
}
func(w Window) Empty() bool{return w.LowerInclusive==w.UpperExclusive}

// Frontier tracks contiguous acknowledgements. It never advances past a missing batch.
type Frontier struct{
 mu sync.Mutex
 next uint64
 done map[uint64]struct{}
}
func NewFrontier(start uint64)*Frontier{return &Frontier{next:start,done:make(map[uint64]struct{})}}
func(f *Frontier) Ack(sequence uint64) (next uint64,err error){
 f.mu.Lock();defer f.mu.Unlock()
 if sequence<f.next{return f.next,nil} // idempotent replay
 // A hard bound avoids unlimited out-of-order completion tracking.
 if sequence-f.next>=1000000{return f.next,errors.New("ack sequence too far ahead")}
 f.done[sequence]=struct{}{}
 for {if _,ok:=f.done[f.next];!ok{break};delete(f.done,f.next);f.next++}
 return f.next,nil
}
func(f *Frontier) Next()uint64{f.mu.Lock();defer f.mu.Unlock();return f.next}
