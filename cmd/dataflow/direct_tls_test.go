package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func syntheticTLSPair(t *testing.T, expired bool) (string,string) {
	t.Helper()
	key,err:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader)
	if err!=nil{t.Fatal(err)}
	now:=time.Now()
	notBefore:=now.Add(-time.Hour)
	notAfter:=now.Add(time.Hour)
	if expired{notBefore=now.Add(-48*time.Hour);notAfter=now.Add(-24*time.Hour)}
	template:=&x509.Certificate{
		SerialNumber:big.NewInt(42), Subject:pkix.Name{CommonName:"localhost"},
		NotBefore:notBefore,NotAfter:notAfter,
		KeyUsage:x509.KeyUsageDigitalSignature,
		ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:[]string{"localhost"},
	}
	der,err:=x509.CreateCertificate(rand.Reader,template,template,&key.PublicKey,key)
	if err!=nil{t.Fatal(err)}
	ecDER,err:=x509.MarshalECPrivateKey(key)
	if err!=nil{t.Fatal(err)}
	dir:=t.TempDir()
	certPath,keyPath:=filepath.Join(dir,"synthetic.crt"),filepath.Join(dir,"synthetic.key")
	if err=os.WriteFile(certPath,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:der}),0600);err!=nil{t.Fatal(err)}
	if err=os.WriteFile(keyPath,pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY",Bytes:ecDER}),0600);err!=nil{t.Fatal(err)}
	return certPath,keyPath
}

func TestDirectTLSDefaultsOffAndRequiresLab(t *testing.T){
	t.Setenv("DATAFLOW_ENABLE_DIRECT_TLS_TEST","false")
	t.Setenv("DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST","false")
	t.Setenv("DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB","false")
	if _,_,config,err:=directTLSFromEnv();err!=nil||config!=nil{t.Fatalf("default plaintext behavior changed: %v",err)}
	t.Setenv("DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST","true")
	if _,_,_,err:=directTLSFromEnv();err==nil{t.Fatal("OIDC route enabled without direct TLS")}
	t.Setenv("DATAFLOW_ENABLE_DIRECT_TLS_TEST","true")
	if _,_,_,err:=directTLSFromEnv();err==nil{t.Fatal("TLS enabled on unapproved lab")}
	t.Setenv("DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB","true")
	if _,_,_,err:=directTLSFromEnv();err==nil{t.Fatal("missing TLS certificate/key accepted")}
}

func TestDirectTLSSyntheticCertificateAndKey(t *testing.T){
	certPath,keyPath:=syntheticTLSPair(t,false)
	t.Setenv("DATAFLOW_ENABLE_DIRECT_TLS_TEST","true")
	t.Setenv("DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB","true")
	t.Setenv("DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST","true")
	t.Setenv("DATAFLOW_DIRECT_TLS_CERT_FILE",certPath)
	t.Setenv("DATAFLOW_DIRECT_TLS_KEY_FILE",keyPath)
	cert,key,config,err:=directTLSFromEnv()
	if err!=nil{t.Fatal(err)}
	if cert!=certPath||key!=keyPath||config==nil||config.MinVersion!=tls.VersionTLS13||len(config.Certificates)!=1{
		t.Fatal("unexpected secure server TLS config")
	}
	// A certificate/key pair mismatch must fail startup without a network call.
	_,differentKey:=syntheticTLSPair(t,false)
	t.Setenv("DATAFLOW_DIRECT_TLS_KEY_FILE",differentKey)
	if _,_,_,err=directTLSFromEnv();err==nil{t.Fatal("mismatched TLS key accepted")}
	t.Setenv("DATAFLOW_DIRECT_TLS_KEY_FILE",keyPath)
	if err=os.Chmod(keyPath,0644);err!=nil{t.Fatal(err)}
	if _,_,_,err=directTLSFromEnv();err==nil{t.Fatal("world-readable TLS private key accepted")}
}

func TestDirectTLSRejectsExpiredSyntheticCertificate(t *testing.T){
	cert,key:=syntheticTLSPair(t,true)
	t.Setenv("DATAFLOW_ENABLE_DIRECT_TLS_TEST","true")
	t.Setenv("DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB","true")
	t.Setenv("DATAFLOW_DIRECT_TLS_CERT_FILE",cert)
	t.Setenv("DATAFLOW_DIRECT_TLS_KEY_FILE",key)
	if _,_,_,err:=directTLSFromEnv();err==nil{t.Fatal("expired TLS certificate accepted")}
}
