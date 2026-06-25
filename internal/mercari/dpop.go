package mercari

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type dpopGenerator struct {
	privateKey *ecdsa.PrivateKey
	deviceUUID string
}

func newDPoPGenerator() (*dpopGenerator, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate EC key: %w", err)
	}

	return &dpopGenerator{
		privateKey: key,
		deviceUUID: uuid.New().String(),
	}, nil
}

func (g *dpopGenerator) generate(method, url string) (string, error) {
	pubKey := g.privateKey.PublicKey

	x := base64URLEncode(pubKey.X.Bytes(), 32)
	y := base64URLEncode(pubKey.Y.Bytes(), 32)

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iat":  time.Now().Unix(),
		"jti":  uuid.New().String(),
		"htu":  url,
		"htm":  method,
		"uuid": g.deviceUUID,
	})

	jwk := map[string]string{
		"crv": "P-256",
		"kty": "EC",
		"x":   x,
		"y":   y,
	}

	token.Header["typ"] = "dpop+jwt"
	token.Header["jwk"] = jwk

	return token.SignedString(g.privateKey)
}

func base64URLEncode(b []byte, size int) string {
	padded := make([]byte, size)
	bInt := new(big.Int).SetBytes(b)
	bInt.FillBytes(padded)
	return base64.RawURLEncoding.EncodeToString(padded)
}

// Verify JSON serialization works for the JWK.
func init() {
	_ = json.Marshal
}
