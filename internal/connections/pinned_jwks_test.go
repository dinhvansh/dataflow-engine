package connectionsecurity

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func syntheticPinnedJWK(key *rsa.PrivateKey, kid string) pinnedJWK {
	return pinnedJWK{
		Kty: "RSA", Kid: kid, Alg: "RS256", Use: "sig",
		N: base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		E: "AQAB",
	}
}

func writeSyntheticJWKS(t *testing.T, path string, keys []pinnedJWK) {
	t.Helper()
	b, err := json.Marshal(pinnedJWKS{Keys: keys})
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, b, 0600); err != nil { t.Fatal(err) }
}

func TestPinnedJWKSKeyRolloverSynthetic(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil { t.Fatal(err) }
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil { t.Fatal(err) }
	file := filepath.Join(t.TempDir(), "operator-pinned.jwks")
	oldJWK := syntheticPinnedJWK(oldKey, "old-key")
	newJWK := syntheticPinnedJWK(newKey, "new-key")
	writeSyntheticJWKS(t, file, []pinnedJWK{oldJWK, newJWK})

	keys, err := LoadPinnedJWKSFile(file)
	if err != nil || len(keys) != 2 { t.Fatalf("overlapping keys: %v", err) }
	issuer := "https://idp.example.invalid/tenant"
	audience := "dataflow-api"
	verifier, err := NewRS256Verifier(issuer, audience, keys)
	if err != nil { t.Fatal(err) }
	now := time.Now().UTC()
	claims := map[string]any{
		"iss": issuer, "sub": "same-verified-subject", "aud": audience,
		"iat": now.Add(-time.Minute).Unix(), "nbf": now.Add(-time.Minute).Unix(),
		"exp": now.Add(5*time.Minute).Unix(),
	}
	oldJWT := signedSyntheticJWT(t, oldKey, map[string]any{"alg":"RS256","kid":"old-key"}, claims)
	newJWT := signedSyntheticJWT(t, newKey, map[string]any{"alg":"RS256","kid":"new-key"}, claims)
	first, err := verifier.Verify(oldJWT)
	if err != nil { t.Fatal(err) }
	second, err := verifier.Verify(newJWT)
	if err != nil || first.ActorID() != second.ActorID() { t.Fatalf("rollover changed actor: %v", err) }

	// Explicit operator replacement + restart: removal withdraws trust. The
	// previous verifier is an immutable snapshot; it is never hot-reloaded
	// from a JWT header or a remotely supplied JWKS URI.
	writeSyntheticJWKS(t, file, []pinnedJWK{newJWK})
	rotated, err := LoadPinnedJWKSFile(file)
	if err != nil { t.Fatal(err) }
	next, err := NewRS256Verifier(issuer, audience, rotated)
	if err != nil { t.Fatal(err) }
	if _,err = next.Verify(oldJWT); !errors.Is(err, ErrForbidden) { t.Fatal("retired key still trusted") }
	if got,err := next.Verify(newJWT); err != nil || got.ActorID() != first.ActorID() { t.Fatal("new key lost identity") }
}

func TestPinnedJWKSRejectsUnsafeKeysAndFiles(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil { t.Fatal(err) }
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil { t.Fatal(err) }
	good := syntheticPinnedJWK(key, "operator-key")
	file := filepath.Join(t.TempDir(), "pins.json")
	check := func(name string, value []byte) {
		t.Helper()
		if err := os.WriteFile(file, value, 0600); err != nil { t.Fatal(err) }
		if _,err := LoadPinnedJWKSFile(file); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: invalid JWKS accepted: %v", name, err)
		}
	}
	for _,tc := range []struct{name string; keys []pinnedJWK}{
		{"no keys", nil},
		{"duplicate kid", []pinnedJWK{good, good}},
		{"unsupported algorithm", []pinnedJWK{func()pinnedJWK{p:=good;p.Alg="RS512";return p}()}},
		{"encryption key", []pinnedJWK{func()pinnedJWK{p:=good;p.Use="enc";return p}()}},
		{"wrong kty", []pinnedJWK{func()pinnedJWK{p:=good;p.Kty="oct";return p}()}},
		{"unapproved exponent", []pinnedJWK{func()pinnedJWK{p:=good;p.E="Aw";return p}()}},
		{"bad kid", []pinnedJWK{func()pinnedJWK{p:=good;p.Kid="../bad key";return p}()}},
		{"weak RSA", []pinnedJWK{syntheticPinnedJWK(weak,"weak")}},
		{"invalid modulus", []pinnedJWK{func()pinnedJWK{p:=good;p.N="not-base64!";return p}()}},
	}{
		payload,err:=json.Marshal(pinnedJWKS{Keys:tc.keys})
		if err!=nil{t.Fatal(err)}
		check(tc.name,payload)
	}
	check("private key parameter",[]byte(`{"keys":[{"kty":"RSA","kid":"operator-key","alg":"RS256","use":"sig","n":"`+good.N+`","e":"AQAB","d":"secret"}]}`))
	check("untrusted jwks URL",[]byte(`{"keys":[{"kty":"RSA","kid":"operator-key","alg":"RS256","use":"sig","n":"`+good.N+`","e":"AQAB","jku":"https://evil.invalid/jwks"}]}`))
	check("unexpected root",[]byte(`{"keys":[],"issuer":"https://evil.invalid"}`))
	check("trailing JSON",[]byte(`{"keys":[]}{"keys":[]}`))
	check("oversized",[]byte(strings.Repeat("A",131073)))

	writeSyntheticJWKS(t,file,[]pinnedJWK{good})
	if err:=os.Chmod(file,0666);err!=nil{t.Fatal(err)}
	if _,err:=LoadPinnedJWKSFile(file);!errors.Is(err,ErrInvalid){t.Fatal("world-writable trust config accepted")}
	if err:=os.Chmod(file,0600);err!=nil{t.Fatal(err)}
	link:=filepath.Join(t.TempDir(),"pins-link.json")
	if err:=os.Symlink(file,link);err!=nil{t.Fatal(err)}
	if _,err:=LoadPinnedJWKSFile(link);!errors.Is(err,ErrInvalid){t.Fatal("symlink trust config accepted")}
	if _,err:=LoadPinnedJWKSFile("");!errors.Is(err,ErrInvalid){t.Fatal("empty path accepted")}
	if _,err:=LoadPinnedJWKSFile(filepath.Dir(file));!errors.Is(err,ErrInvalid){t.Fatal("directory accepted")}
}
