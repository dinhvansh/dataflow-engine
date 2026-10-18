package connectionnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

type mockTCPDialer struct{
	calls []string
	succeedAt string
}
func(d *mockTCPDialer)DialContext(_ context.Context,network,address string)(net.Conn,error){
	if network!="tcp"{return nil,errors.New("unexpected network")}
	d.calls=append(d.calls,address)
	if address==d.succeedAt{
		client,server:=net.Pipe()
		_ = server.Close()
		return client,nil
	}
	return nil,errors.New("backend-secret-should-never-leak")
}

func TestPinnedTCPProbeUsesOnlyApprovedIPNotHostname(t *testing.T){
	dns:=&cannedDNS{answers:map[string][]netip.Addr{
		"sql.lab.internal":{addr("10.20.1.11"),addr("10.20.1.12")},
	}}
	policy,err:=NewPolicy([]HostPort{{Host:"sql.lab.internal",Port:1433}},
		[]netip.Prefix{cidr("10.20.0.0/16")},dns)
	if err!=nil{t.Fatal(err)}
	plan,err:=policy.Admit(context.Background(),"sql.lab.internal",1433)
	if err!=nil{t.Fatal(err)}
	// Force DNS answers to rebind AFTER admission: probe MUST use frozen IPs.
	dns.answers["sql.lab.internal"]=[]netip.Addr{addr("169.254.169.254")}
	lookups:=dns.requests
	mock:=&mockTCPDialer{succeedAt:"10.20.1.12:1433"}
	result,err:=ProbePinned(context.Background(),plan,mock)
	if err!=nil||!result.Reachable||result.Attempts!=2{t.Fatalf("probe failed: %+v %v",result,err)}
	if !reflect.DeepEqual(mock.calls,[]string{"10.20.1.11:1433","10.20.1.12:1433"}) {
		t.Fatalf("probe escaped pinned addresses: %v",mock.calls)
	}
	if dns.requests!=lookups{t.Fatal("probe re-resolved DNS")}
}
func TestPinnedTCPProbeFailClosedAndSanitized(t *testing.T){
	mock:=&mockTCPDialer{}
	if _,err:=ProbePinned(context.Background(),Plan{},mock);!errors.Is(err,ErrDenied){t.Fatal("empty plan admitted")}
	if len(mock.calls)!=0{t.Fatal("empty plan triggered dial")}
	plan:=Plan{serverName:"synthetic",port:443,ips:[]netip.Addr{addr("127.0.0.1")}}
	if _,err:=ProbePinned(context.Background(),plan,mock);!errors.Is(err,ErrDenied){t.Fatal("loopback admitted")}
	plan.ips=[]netip.Addr{addr("10.20.1.11")}
	result,err:=ProbePinned(context.Background(),plan,mock)
	if !errors.Is(err,ErrUnreachable)||result.Reachable||result.Attempts!=1{t.Fatalf("unexpected unreachable: %+v %v",result,err)}
	if strings.Contains(err.Error(),"backend-secret"){t.Fatal("raw dial error leaked")}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	if _,err=ProbePinned(ctx,plan,mock);!errors.Is(err,ErrDenied){t.Fatal("canceled context dialed")}
}
func TestPinnedTCPProbeBoundedAttempts(t *testing.T){
	plan:=Plan{serverName:"synthetic",port:8443,
		ips:[]netip.Addr{addr("10.20.1.1"),addr("10.20.1.2"),addr("10.20.1.3"),addr("10.20.1.4"),addr("10.20.1.5")}}
	mock:=&mockTCPDialer{}
	result,err:=ProbePinned(context.Background(),plan,mock)
	if !errors.Is(err,ErrUnreachable)||result.Attempts!=4||len(mock.calls)!=4{
		t.Fatalf("unbounded dial attempts: %+v %v %v",result,err,mock.calls)
	}
}

// A non-network fake blocks until request cancellation, proving caller budget
// is honored without ever opening a real socket.
type cancelDialer struct{}
func(cancelDialer)DialContext(ctx context.Context,_,_ string)(net.Conn,error){
	<-ctx.Done()
	return nil,ctx.Err()
}
func TestPinnedTCPProbeHonorsCallerDeadline(t *testing.T){
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Millisecond)
	defer cancel()
	plan:=Plan{serverName:"synthetic",port:1433,ips:[]netip.Addr{addr("10.20.1.11")}}
	before:=time.Now()
	r,err:=ProbePinned(ctx,plan,cancelDialer{})
	if !errors.Is(err,ErrUnreachable)||r.Reachable||time.Since(before)>time.Second{
		t.Fatalf("probe deadline ignored: %+v %v",r,err)
	}
}
