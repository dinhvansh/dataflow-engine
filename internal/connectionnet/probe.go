package connectionnet

import (
	"context"
	"errors"
	"net"
	"time"
)

// ErrUnreachable deliberately omits hosts, IPs, credentials and driver errors.
var ErrUnreachable=errors.New("approved endpoint unavailable")

// Dialer is provided by trusted runtime infrastructure; no user can supply
// a custom dialer or redirect this probe to arbitrary IP addresses.
type Dialer interface {
	DialContext(ctx context.Context,network,address string)(net.Conn,error)
}

type ProbeResult struct {
	Reachable bool `json:"reachable"`
	Attempts int `json:"attempts"`
}

// ProbePinned makes at most four bounded synthetic/infrastructure TCP attempts
// ONLY to the exact already-admitted IP:port snapshot, never DNS hostname.
// It is not a SQL Server or ClickHouse authentication/permission test.
// There is NO HTTP route or call from existing live job runtime.
func ProbePinned(ctx context.Context,plan Plan,dialer Dialer)(ProbeResult,error){
	if ctx==nil||ctx.Err()!=nil||dialer==nil||plan.serverName==""||
		plan.port==0||len(plan.ips)==0||len(plan.ips)>32{
		return ProbeResult{},ErrDenied
	}
	for _,addr:=range plan.ips{if !safeIP(addr){return ProbeResult{},ErrDenied}}
	bounded,cancel:=context.WithTimeout(ctx,3*time.Second)
	defer cancel()
	addresses:=plan.Addresses()
	result:=ProbeResult{}
	for _,address:=range addresses {
		if result.Attempts>=4||bounded.Err()!=nil{break}
		result.Attempts++
		conn,err:=dialer.DialContext(bounded,"tcp",address)
		if err!=nil{continue}
		if conn==nil{continue}
		_ = conn.Close()
		result.Reachable=true
		return result,nil
	}
	return result,ErrUnreachable
}
