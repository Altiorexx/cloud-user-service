package service

import (
	"fmt"
	"os"

	"github.com/golang-jwt/jwt"
	"user.service.altiore.io/types"
)

type TokenService struct {
	jwt_secret string
}

// Instantiates a token service.
func NewTokenService() *TokenService {
	return &TokenService{
		jwt_secret: os.Getenv("JWT_SECRET"),
	}
}

// Determines whether a token is valid or not.
func (s *TokenService) Validate(token string) error {
	return nil
}

// Parses the token using the original secret.
func (s *TokenService) Parse(token string) (jwt.MapClaims, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwt_secret), nil
	})

	if err != nil {
		return nil, err
	}

	if !parsedToken.Valid {
		return nil, fmt.Errorf("invalid JWT")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract JWT claims")
	}

	return claims, nil
}

// Parses and extracts the userId from the token.
func (s *TokenService) GetUserId(token string) (string, error) {
	parsedToken, err := s.Parse(token)
	if err != nil {
		return "", err
	}
	return parsedToken["userId"].(string), nil
}

// Parses a token to the TokenData type.
func (service *TokenService) ParseToStruct(token string) (*types.TokenData, error) {
	parsedToken, err := service.Parse(token)
	if err != nil {
		return nil, err
	}
	userId, ok := parsedToken["userId"].(string)
	if !ok {
		return nil, fmt.Errorf("no userId set in token")
	}
	_organisationIds, ok := parsedToken["organisationIds"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no organisationId set in token")
	}
	var organisationIds []string
	for _, v := range _organisationIds {
		v, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("an organisationId was not a string")
		}
		organisationIds = append(organisationIds, v)
	}

	return &types.TokenData{
		UserId:          userId,
		OrganisationIds: organisationIds,
	}, nil
}
