package sqlserver
import "testing"
func TestKeySnapshotRequiresDB(t *testing.T){
 reader:=&KeySnapshotReader{BatchSize:100,Schema:"Erp",Table:"PartTran"}
 if err:=reader.Read(nil,nil);err==nil{t.Fatal("expected nil DB rejection")}
}
