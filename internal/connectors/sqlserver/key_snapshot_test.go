package sqlserver
import "testing"
func TestKeySnapshotRequiresDB(t *testing.T){
 reader:=&KeySnapshotReader{BatchSize:100,Schema:"Erp",Table:"PartTran"}
 if err:=reader.Read(nil,nil);err==nil{t.Fatal("expected nil DB rejection")}
}

// No silent padding/truncation of rowversion is allowed.
func TestSnapshotRowVersionBinaryWidth(t *testing.T){
 cases:=[]struct{raw []byte;want string;ok bool}{
  {[]byte{0,0,0,0,0,0,0,1},"0x0000000000000001",true},
  {[]byte{0x80,0,0,0,0,0,0,0xff},"0x80000000000000FF",true},
  {[]byte{0,0,0,0},"",false},
  {nil,"",false},
  {[]byte{0,0,0,0,0,0,0,0,0},"",false},
 }
 for _,c:=range cases{
  actual,err:=snapshotRowVersion(c.raw)
  if (err==nil)!=c.ok||actual!=c.want{t.Fatalf("input %x: got %q err %v",c.raw,actual,err)}
 }
}
