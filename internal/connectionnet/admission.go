// Package connectionnet provides a fail-closed, OFFLINE connection admission
// boundary. It NEVER performs a socket dial or contacts a database by itself.
package connectionnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrDenied = errors.New("endpoint not permitted")
	ErrResolve = errors.New("endpoint resolution unavailable")
)

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type HostPort struct {
	Host string
	Port uint16
}

// Policy is assembled ONLY from trusted operator deployment configuration.
// No client-provided host, port, CIDR or DNS resolver may modify it.
type Policy struct {
	rules map[string]map[uint16]bool
	networks []netip.Prefix
	resolver Resolver
}

var validHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

func cleanHost(host string) (string, bool) {
	if len(host)<1||len(host)>253||host!=strings.ToLower(host)||!validHost.MatchString(host)||
		strings.HasSuffix(host,".")||strings.Contains(host,"..")||strings.HasPrefix(host,"-")||
		strings.Contains(host,":")||strings.Contains(host,"/") {
		return "",false
	}
	for _,label:=range strings.Split(host,".") {
		if len(label)==0||len(label)>63||label[0]=='-'||label[len(label)-1]=='-' {return "",false}
	}
	return host,true
}

func NewPolicy(approved []HostPort, allowedCIDRs []netip.Prefix, resolver Resolver) (*Policy,error) {
	if resolver==nil||len(approved)==0||len(approved)>128||len(allowedCIDRs)==0||len(allowedCIDRs)>128 {
		return nil,ErrDenied
	}
	p:=&Policy{rules:make(map[string]map[uint16]bool),networks:make([]netip.Prefix,0,len(allowedCIDRs)),resolver:resolver}
	for _,entry:=range approved {
		host,ok:=cleanHost(entry.Host)
		if !ok||entry.Port==0||host=="localhost"||strings.HasSuffix(host,".localhost") {return nil,ErrDenied}
		if p.rules[host]==nil {p.rules[host]=map[uint16]bool{}}
		p.rules[host][entry.Port]=true
	}
	for _,prefix:=range allowedCIDRs {
		if !prefix.IsValid()||prefix.Addr().Is4In6()||prefix!=prefix.Masked() ||
\t\t\t(prefix.Addr().Is4() && prefix.Bits()<8) || (prefix.Addr().Is6() && prefix.Bits()<32) {return nil,ErrDenied}
		p.networks=append(p.networks,prefix)
	}
	return p,nil
}

func safeIP(addr netip.Addr) bool {
	if !addr.IsValid()||addr.Is4In6()||addr.Zone()!="" ||
		addr.IsUnspecified()||addr.IsLoopback()||addr.IsLinkLocalUnicast()||
		addr.IsLinkLocalMulticast()||addr.IsMulticast()||addr.IsInterfaceLocalMulticast() {
		return false
	}
	// 255.255.255.255 and IPv4 broadcast are not usable as connections.
	if addr.Is4() && addr==netip.MustParseAddr("255.255.255.255") {return false}
	return true
}

// Plan is an immutable, snapshot-pinned, server-side selection. Database
// connectors must dial the returned IP:port directly; MUST NOT re-resolve
// hostname or follow HTTP redirects. TLS SNI/Host must use ServerName().
type Plan struct {
	serverName string
	port uint16
	ips []netip.Addr
}

func (p Plan) ServerName() string {return p.serverName}
func (p Plan) Port() uint16 {return p.port}
func (p Plan) Addresses() []string {
	copyIPs:=make([]string,len(p.ips))
	for i,addr:=range p.ips {copyIPs[i]=net.JoinHostPort(addr.String(),strconv.Itoa(int(p.port)))}
	return copyIPs
}

// Admit resolves ALL DNS answers and rejects the request if ANY is not in an
// explicitly trusted CIDR. Selecting just one allowed answer is unsafe against
// DNS rebinding and mixed private/public answer sets.
func (p *Policy) Admit(ctx context.Context,host string,port uint16)(Plan,error){
	if p==nil||ctx==nil||ctx.Err()!=nil||p.resolver==nil||port==0{return Plan{},ErrDenied}
	norm,ok:=cleanHost(host)
	if !ok||!p.rules[norm][port]{return Plan{},ErrDenied}
	var addresses []netip.Addr
	if literal,err:=netip.ParseAddr(host);err==nil {
		addresses=[]netip.Addr{literal}
	} else {
		resolved,err:=p.resolver.LookupNetIP(ctx,"ip",host)
		if err!=nil{return Plan{},ErrResolve}
		addresses=resolved
	}
	if len(addresses)==0||len(addresses)>32{return Plan{},ErrDenied}
	unique:=map[netip.Addr]bool{}
	for _,addr:=range addresses {
		if !safeIP(addr){return Plan{},ErrDenied}
		approved:=false
		for _,prefix:=range p.networks {if prefix.Contains(addr){approved=true;break}}
		if !approved{return Plan{},ErrDenied}
		unique[addr]=true
	}
	ips:=make([]netip.Addr,0,len(unique))
	for addr:=range unique{ips=append(ips,addr)}
	sort.Slice(ips,func(i,j int)bool{return ips[i].Compare(ips[j])<0})
	return Plan{serverName:host,port:port,ips:ips},nil
}
