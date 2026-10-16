package cursor

import(
 "sync"
 "testing"
)
func TestRowVersion(t *testing.T){
 v,err:=ParseRowVersion("0x000000000001A2B3");if err!=nil{t.Fatal(err)}
 if v.String()!="0x000000000001A2B3"{t.Fatalf("wrong round trip %s",v)}
 cp,err:=FromBytes(v.Bytes());if err!=nil||cp!=v{t.Fatal("binary round trip failed")}
 raw:=v.Bytes();raw[0]=123;if v.Bytes()[0]==123{t.Fatal("mutated cursor")}
 next,_:=ParseRowVersion("0x000000000001A2B4");if v.Compare(next)>=0{t.Fatal("cursor sort failed")}
 for _,s:=range []string{"","0xA","0xZZZZZZZZZZZZZZZZ","abcdef01234567890","0x000000000001A2B30"}{if _,err:=ParseRowVersion(s);err==nil{t.Fatalf("accepted %q",s)}}
}
func TestWindow(t *testing.T){
 a,_:=ParseRowVersion("0000000000000001");b,_:=ParseRowVersion("0000000000000002")
 if (Window{LowerInclusive:b,UpperExclusive:a}).Validate()==nil{t.Fatal("allowed regressing range")}
 if (Window{LowerInclusive:a,UpperExclusive:b}).Validate()!=nil{t.Fatal("valid window rejected")}
 if !(Window{LowerInclusive:a,UpperExclusive:a}).Empty(){t.Fatal("equal bound not empty")}
}
func TestContiguousAck(t *testing.T){
 f:=NewFrontier(0)
 if n,_:=f.Ack(2);n!=0{t.Fatal(n)}
 if n,_:=f.Ack(0);n!=1{t.Fatal(n)}
 if n,_:=f.Ack(1);n!=3{t.Fatal(n)}
 if n,_:=f.Ack(2);n!=3{t.Fatal(n)}
 if _,err:=f.Ack(1000003);err==nil{t.Fatal("accepted unbounded ack")}
}
func TestConcurrentAck(t *testing.T){
 f:=NewFrontier(0);var wg sync.WaitGroup
 for i:=uint64(0);i<1000;i++{wg.Add(1);go func(n uint64){defer wg.Done();_,_=f.Ack(n)}(i)}
 wg.Wait();if f.Next()!=1000{t.Fatalf("frontier=%d",f.Next())}
}
