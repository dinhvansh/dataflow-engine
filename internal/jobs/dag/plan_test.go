package dag

import("reflect";"testing")
func sample()Plan{return Plan{Nodes:[]Node{
 {ID:"a",JobID:"sync_a"},
 {ID:"b",JobID:"sync_b"},
 {ID:"transform",JobID:"sql_transform",Requires:[]string{"a","b"}},
 {ID:"quality",JobID:"quality_check",Requires:[]string{"transform"}},
}}}
func TestForkJoin(t *testing.T){
 p:=sample()
 ready,err:=p.Ready(nil);if err!=nil||!reflect.DeepEqual(ready,[]string{"a","b"}){t.Fatalf("%v %v",ready,err)}
 ready,err=p.Ready(map[string]Status{"a":Succeeded,"b":Running});if err!=nil||len(ready)!=0{t.Fatalf("%v %v",ready,err)}
 ready,err=p.Ready(map[string]Status{"a":Succeeded,"b":Succeeded});if err!=nil||!reflect.DeepEqual(ready,[]string{"transform"}){t.Fatalf("%v %v",ready,err)}
}
func TestCycleAndMissing(t *testing.T){
 tests:=[]Plan{
 {Nodes:[]Node{{ID:"a",JobID:"a",Requires:[]string{"b"}},{ID:"b",JobID:"b",Requires:[]string{"a"}}}},
 {Nodes:[]Node{{ID:"a",JobID:"a",Requires:[]string{"missing"}}}},
 {Nodes:[]Node{{ID:"a",JobID:"a"},{ID:"a",JobID:"dup"}}},
 }
 for _,p:=range tests{if p.Validate()==nil{t.Fatalf("invalid plan accepted: %#v",p)}}
}
func TestBlocked(t *testing.T){
 p:=sample();state:=map[string]Status{"a":Failed,"b":Succeeded}
 blocked,err:=p.Blocked(state)
 if err!=nil||!reflect.DeepEqual(blocked,[]string{"transform"}){t.Fatalf("%v %v",blocked,err)}
}
