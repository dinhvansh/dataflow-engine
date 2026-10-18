package connectionsecurity

import (
	"bytes"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"os"
)

// pinnedJWK deliberately supports ONLY operator-curated RSA verification keys.
// Unexpected key material, URLs (jku/x5u), private parameters and algorithms
// are rejected rather than silently ignored.
type pinnedJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type pinnedJWKS struct {
	Keys []pinnedJWK `json:"keys"`
}

// LoadPinnedJWKSFile loads a FIXED, operator-provisioned, local, read-only
// signing-key set. It performs no network/JWKS discovery and cannot load
// signing keys from token headers. Rolling keys requires an operator-controlled
// file replacement and service restart. Old keys stop being trusted on reload.
// Public keys are not secrets, but untrusted writable config is unsafe.
func LoadPinnedJWKSFile(path string) (map[string]*rsa.PublicKey, error) {
	if path == "" {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 ||
		info.Size() <= 0 || info.Size() > 131072 {
		return nil, ErrInvalid
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 || len(b) > 131072 {
		return nil, ErrInvalid
	}
	var bundle pinnedJWKS
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || decoder.Decode(new(any)) != io.EOF ||
		len(bundle.Keys) == 0 || len(bundle.Keys) > 32 {
		return nil, ErrInvalid
	}
	keys := make(map[string]*rsa.PublicKey, len(bundle.Keys))
	for _, jwk := range bundle.Keys {
		if jwk.Kty != "RSA" || jwk.Alg != "RS256" || jwk.Use != "sig" ||
			!keyIDPattern.MatchString(jwk.Kid) || jwk.E != "AQAB" {
			return nil, ErrInvalid
		}
		n, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil || len(n) == 0 || n[0] == 0 ||
			base64.RawURLEncoding.EncodeToString(n) != jwk.N {
			return nil, ErrInvalid
		}
		modulus := new(big.Int).SetBytes(n)
		if modulus.BitLen() < 2048 || modulus.BitLen() > 4096 || modulus.Bit(0) == 0 {
			return nil, ErrInvalid
		}
		if _, duplicate := keys[jwk.Kid]; duplicate {
			return nil, ErrInvalid
		}
		keys[jwk.Kid] = &rsa.PublicKey{N: modulus, E: 65537}
	}
	return keys, nil
}
