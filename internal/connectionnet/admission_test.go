package connectionnet

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

type cannedDNS struct {
	answers map[string][]netip.Addr
	requests int
	fail bool
}
func (f *cannedDNS) LookupNetIP(_ context.Context, network, host string)([]netip.Addr,error){
	f.requests++
	if f.fail||network!="ip"{return nil,errors.New("synthetic DNS offline")}
	return append([]netip.Addr(nil),f.answers[host]...),nil
}
func addr(s string)netip.Addr{return netip.MustParseAddr(s)}
func cidr(s string)netip.Prefix{return netip.MustParsePrefix(s)}

func TestEndpointAdmissionAllowlistAndPinnedAddresses(t *testing.T){
	dns:=&cannedDNS{answers:map[string][]netip.Addr{
		"sql.lab.internal":{addr("10.20.1.11"),addr("10.20.1.12"),addr("10.20.1.11")},
		"ch.lab.internal":{addr("10.20.2.50")},
		"mixed.lab.internal":{addr("10.20.1.11"),addr("169.254.169.254")},
	}}
	p,err:=NewPolicy([]HostPort{
		{Host:"sql.lab.internal",Port:1433},
		{Host:"ch.lab.internal",Port:8443},
		{Host:"mixed.lab.internal",Port:1433},
	},[]netip.Prefix{cidr("10.20.0.0/16")},dns)
	if err!=nil{t.Fatal(err)}
	plan,err:=p.Admit(context.Background(),"sql.lab.internal",1433)
	if err!=nil{t.Fatal(err)}
	want:=[]string{"10.20.1.11:1433","10.20.1.12:1433"}
	if got:=plan.Addresses();!reflect.DeepEqual(got,want){t.Fatalf("pinned address mismatch: %v",got)}
	if plan.ServerName()!="sql.lab.internal"||plan.Port()!=1433{t.Fatal("TLS SNI/port changed")}
	// Copy protection: mutations to returned addresses do not alter plan.
	actual:=plan.Addresses()
	actual[0]="127.0.0.1:1"
	if !reflect.DeepEqual(plan.Addresses(),want){t.Fatal("mutable pinned plan")}
	// No network resolver lookup is made for non-allowlisted destinations.
	count:=dns.requests
	if _,err=p.Admit(context.Background(),"attacker.internal",1433);!errors.Is(err,ErrDenied){t.Fatalf("new host admitted: %v",err)}
	if _,err=p.Admit(context.Background(),"sql.lab.internal",3306);!errors.Is(err,ErrDenied){t.Fatalf("extra port admitted: %v",err)}
	if dns.requests!=count{t.Fatal("unapproved host triggered DNS")}
	if _,err=p.Admit(context.Background(),"mixed.lab.internal",1433);!errors.Is(err,ErrDenied){
		t.Fatalf("mixed approved + metadata IP admitted: %v",err)
	}
}

func TestEndpointAdmissionRejectsRebindingAndReservedAddresses(t *testing.T) {
	dns:=&cannedDNS{answers:map[string][]netip.Addr{}}
	p,err:=NewPolicy([]HostPort{{Host:"db.lab.internal",Port:1433}},[]netip.Prefix{
		cidr("10.10.0.0/16"),cidr("203.0.113.0/24"),cidr("2001:db8:1::/48"),
	},dns)
	if err!=nil{t.Fatal(err)}
	for _,s:=range []string{
		"127.0.0.1", "0.0.0.0", "255.255.255.255", "169.254.169.254",
		"224.0.0.1", "fe80::1", "ff02::1", "::1", "::ffff:10.10.1.1",
		"203.0.114.10", "172.20.10.2",
	}{
		dns.answers["db.lab.internal"]=[]netip.Addr{addr(s)}
		if _,err=p.Admit(context.Background(),"db.lab.internal",1433);!errors.Is(err,ErrDenied){
			t.Fatalf("unsafe/unknown IP admitted %s: %v",s,err)
		}
	}
	dns.answers["db.lab.internal"]=[]netip.Addr{addr("2001:db8:1::101")}
	if _,err=p.Admit(context.Background(),"db.lab.internal",1433);err!=nil{t.Fatalf("operator-pinned IPv6 rejected: %v",err)}
	dns.answers["db.lab.internal"]=[]netip.Addr{addr("10.10.1.2"),addr("127.0.0.1")}
	if _,err=p.Admit(context.Background(),"db.lab.internal",1433);!errors.Is(err,ErrDenied){t.Fatal("DNS rebinding answer set admitted")}
	dns.fail=true
	if _,err=p.Admit(context.Background(),"db.lab.internal",1433);!errors.Is(err,ErrResolve){t.Fatalf("resolver error allowed: %v",err)}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	if _,err=p.Admit(ctx,"db.lab.internal",1433);!errors.Is(err,ErrDenied){t.Fatal("canceled request admitted")}
}

func TestEndpointAdmissionRejectsSpoofingAndUnsafeOperatorPolicy(t *testing.T){
	f:=&cannedDNS{answers:map[string][]netip.Addr{}}
	allowed:=[]HostPort{{Host:"erp.lab.internal",Port:1433}}
	good:=[]netip.Prefix{cidr("10.20.0.0/16")}
	for _,h:=range []string{
		"ERP.lab.internal","erp.lab.internal.","erp..lab.internal","-erp.lab.internal",
		"erp.lab.internal:1433","https://erp.lab.internal","erp.lab.internal/path",
		"erp.lab.internal@127.0.0.1","localhost","sub.localhost",
		"erp.lab.internal\n","erp_lab_internal",
	}{
		if _,err:=NewPolicy([]HostPort{{Host:h,Port:1433}},good,f);!errors.Is(err,ErrDenied){t.Fatalf("unsafe operator host accepted %q: %v",h,err)}
	}
	for _,bad:=range []struct{
		hosts []HostPort
		cidrs []netip.Prefix
		resolver Resolver
	}{
		{nil,good,f},{allowed,nil,f},{allowed,good,nil},
		{[]HostPort{{Host:"erp.lab.internal",Port:0}},good,f},
		{allowed,[]netip.Prefix{cidr("10.20.0.1/24")},f},
		{allowed,[]netip.Prefix{cidr("::ffff:10.10.0.0/120")},f},
		{allowed,[]netip.Prefix{cidr("0.0.0.0/0")},f},
		{allowed,[]netip.Prefix{cidr("::/0")},f},
	}{
		if _,err:=NewPolicy(bad.hosts,bad.cidrs,bad.resolver);!errors.Is(err,ErrDenied){t.Fatalf("unsafe operator policy admitted: %+v %v",bad,err)}
	}
}

func TestEndpointAdmissionFailsClosedIfDNSAnswersTooMany(t *testing.T){
	addrs:=make([]netip.Addr,33)
	for i:=range addrs {addrs[i]=addr("10.20.1.1")}
	dns:=&cannedDNS{answers:map[string][]netip.Addr{"db.lab.internal":addrs}}
	p,err:=NewPolicy([]HostPort{{Host:"db.lab.internal",Port:1433}},[]netip.Prefix{cidr("10.20.0.0/16")},dns)
	if err!=nil{t.Fatal(err)}
	if _,err=p.Admit(context.Background(),"db.lab.internal",1433);!errors.Is(err,ErrDenied){t.Fatalf("unbounded DNS accepted: %v",err)}
}
