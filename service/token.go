package service

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"user.service.altiore.io/types"
)

type TokenService interface {
	NewToken(audience string) (string, error)
	CheckToken(token string) error
}

type TokenServiceImpl struct {
	service_token_secret string
	issuer               string
	internalList         []string
}

type TokenServiceOpts struct{}

func NewTokenService(opts *TokenServiceOpts) TokenService {
	return &TokenServiceImpl{
		service_token_secret: os.Getenv("SERVICE_TOKEN_SECRET"),
		issuer:               os.Getenv("SERVICE_TOKEN_ISSUER"),
	}
}

// Generates a new JWT for the specified audience.
func (service *TokenServiceImpl) NewToken(audience string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": service.issuer,
		"aud": audience,
		"exp": time.Minute * 5,
	})
	signedToken, err := token.SignedString([]byte(service.service_token_secret))
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func (service *TokenServiceImpl) CheckToken(token string) error {
	_token, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %+v", t.Header["alg"])
		}
		return []byte(service.service_token_secret), nil
	})
	if err != nil {
		if err == jwt.ErrSignatureInvalid {
			return fmt.Errorf("invalid token signature")
		}
		return fmt.Errorf("error parsing token")
	}

	if !_token.Valid {
		return types.ErrInvalidToken
	}

	// check iss, aud (issuer, audience)
	// should match sender and receiver
	// include relevant header as param

	return nil
}
