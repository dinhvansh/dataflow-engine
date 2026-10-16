package sqlserver

import (
 "testing"
 "time"
)

func TestQuoteIdentifier(t *testing.T) {
 for _, s := range []string{"Erp", "PartTran", "SysRevID"} {
  got, err := quote(s)
  if err != nil || got != "["+s+"]" { t.Fatalf("%q: %q %v",s,got,err) }
 }
 for _, s := range []string{"", "Erp.PartTran", "Part];DROP TABLE X", "a b"} {
  if _,err:=quote(s);err==nil { t.Fatalf("unsafe identifier accepted: %q",s) }
 }
}

func TestNormalizeSQLServerValues(t *testing.T) {
 raw:=[]byte{0x7A,0x7F,0x8A,0x4D,0x43,0xF3,0xB7,0xAF,0x17,0xA9,0x10,0x3D,0x63,0xE7,0xAA,0x4E}
 if got:=normalizeValue(raw,"UNIQUEIDENTIFIER");got!="4D8A7F7A-F343-AFB7-17A9-103D63E7AA4E"{t.Fatalf("uuid=%v",got)}
 stamp:=time.Date(2024,1,2,3,4,5,123000000,time.UTC)
 if got:=normalizeValue(stamp,"DATE");got!="2024-01-02"{t.Fatalf("date=%v",got)}
 if got:=normalizeValue(stamp,"DATETIME2");got!="2024-01-02 03:04:05.123"{t.Fatalf("datetime=%v",got)}
}
