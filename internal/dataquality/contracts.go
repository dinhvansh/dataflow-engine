// Package dataquality validates immutable, synthetic metric evidence before
// allowing a data release. It does not query any real databases by itself.
package dataquality

import (
	"errors"
	"math"
	"regexp"
)

var ErrInvalid=errors.New("invalid data quality contract")
var ErrBlocked=errors.New("data quality gate blocked")

type Kind string
const (
	MinRows Kind = "MIN_ROWS"
	MaxNullRatio Kind = "MAX_NULL_RATIO"
	MaxDuplicateKeys Kind = "MAX_DUPLICATE_KEYS"
	MaxAgeSeconds Kind = "MAX_AGE_SECONDS"
	MaxReconciliationDelta Kind = "MAX_RECONCILIATION_DELTA"
	RequiredColumn Kind = "REQUIRED_COLUMN"
)

type Check struct {
	ID string `json:"id"`
	Kind Kind `json:"kind"`
	Column string `json:"column,omitempty"`
	Limit float64 `json:"limit"`
}
type Contract struct{ Checks []Check `json:"checks"` }

// Evidence MUST be produced by a future trusted read-only metrics adapter.
// Nil metric pointers are deliberately treated as MISSING, never as 0.
type Evidence struct {
	RowCount *int64
	Columns map[string]bool
	NullByColumn map[string]int64
	DuplicateKeys *int64
	AgeSeconds *int64
	ReconciliationDelta *float64
}

type CheckResult struct{
	ID string `json:"id"`
	Passed bool `json:"passed"`
	Reason string `json:"reason,omitempty"`
}
type Result struct{
	Passed bool `json:"passed"`
	Checks []CheckResult `json:"checks"`
}

var ident=regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

func (c Contract) Validate()error{
	if len(c.Checks)==0||len(c.Checks)>64{return ErrInvalid}
	seen:=map[string]bool{}
	for _,ch:=range c.Checks {
		if !ident.MatchString(ch.ID)||seen[ch.ID]||
			math.IsNaN(ch.Limit)||math.IsInf(ch.Limit,0)||ch.Limit<0{
			return ErrInvalid
		}
		seen[ch.ID]=true
		switch ch.Kind {
		case MinRows,MaxDuplicateKeys,MaxAgeSeconds:
			if ch.Column!=""||ch.Limit>1e12||math.Trunc(ch.Limit)!=ch.Limit{return ErrInvalid}
		case MaxNullRatio:
			if !ident.MatchString(ch.Column)||ch.Limit>1{return ErrInvalid}
		case MaxReconciliationDelta:
			if ch.Column!=""||ch.Limit>1{return ErrInvalid}
		case RequiredColumn:
			if !ident.MatchString(ch.Column)||ch.Limit!=0{return ErrInvalid}
		default: return ErrInvalid
		}
	}
	return nil
}

func validCounter(v *int64)bool{return v!=nil&&*v>=0}

func Evaluate(c Contract,e Evidence)(Result,error){
	if err:=c.Validate();err!=nil{return Result{},err}
	out:=Result{Passed:true,Checks:make([]CheckResult,0,len(c.Checks))}
	for _,ch:=range c.Checks{
		r:=CheckResult{ID:ch.ID,Passed:false,Reason:"evidence_unavailable"}
		switch ch.Kind {
		case MinRows:
			if validCounter(e.RowCount){
				r.Passed=float64(*e.RowCount)>=ch.Limit
				r.Reason="threshold_failed"
			}
		case RequiredColumn:
			if e.Columns!=nil{
				r.Passed=e.Columns[ch.Column]
				r.Reason="schema_mismatch"
			}
		case MaxNullRatio:
			count,ok:=e.NullByColumn[ch.Column]
			if validCounter(e.RowCount)&&*e.RowCount>0&&ok&&count>=0&&count<=*e.RowCount&&e.Columns[ch.Column]{
				r.Passed=float64(count)/float64(*e.RowCount)<=ch.Limit
				r.Reason="threshold_failed"
			}
		case MaxDuplicateKeys:
			if validCounter(e.RowCount)&&validCounter(e.DuplicateKeys)&&*e.DuplicateKeys<=*e.RowCount{
				r.Passed=float64(*e.DuplicateKeys)<=ch.Limit
				r.Reason="threshold_failed"
			}
		case MaxAgeSeconds:
			if validCounter(e.AgeSeconds){
				r.Passed=float64(*e.AgeSeconds)<=ch.Limit
				r.Reason="stale_data"
			}
		case MaxReconciliationDelta:
			if e.ReconciliationDelta!=nil&&!math.IsNaN(*e.ReconciliationDelta)&&
				!math.IsInf(*e.ReconciliationDelta,0)&&*e.ReconciliationDelta>=0{
				r.Passed=*e.ReconciliationDelta<=ch.Limit
				r.Reason="reconciliation_failed"
			}
		}
		if r.Passed{r.Reason=""}else{out.Passed=false}
		out.Checks=append(out.Checks,r)
	}
	if !out.Passed{return out,ErrBlocked}
	return out,nil
}
