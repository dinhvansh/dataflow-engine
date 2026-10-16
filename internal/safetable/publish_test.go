package safetable
import("testing")
func TestNaming(t *testing.T){
 if StageName("01234567-89ab-cdef-0123-456789abcdef")!="dft_stage_0123456789abcdef0123456789abcdef"{t.Fatal("incorrect stage")}
 if SafeNames("revenue","dft_stage_0123456789abcdef0123456789abcdef")!=nil{t.Fatal("valid names rejected")}
 for _,bad:=range []string{"../secret","A","x;DROP",""}{if SafeNames(bad,"dft_stage_0123456789abcdef0123456789abcdef")==nil{t.Fatalf("accepted %s",bad)}}
 p:=Publication{Target:"dft_revenue",Stage:"dft_stage_0123456789abcdef0123456789abcdef",StageUUID:"abc"}
 if p.validate()!=nil{t.Fatal("valid publication rejected")}
 p.Target="scratch.secret";if p.validate()==nil{t.Fatal("unsafe target accepted")}
}
