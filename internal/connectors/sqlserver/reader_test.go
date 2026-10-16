package sqlserver

import "testing"

func TestQuoteIdentifier(t *testing.T) {
 for _, s := range []string{"Erp", "PartTran", "SysRevID"} {
  got, err := quote(s)
  if err != nil || got != "["+s+"]" { t.Fatalf("%q: %q %v",s,got,err) }
 }
 for _, s := range []string{"", "Erp.PartTran", "Part];DROP TABLE X", "a b"} {
  if _,err:=quote(s);err==nil { t.Fatalf("unsafe identifier accepted: %q",s) }
 }
}
