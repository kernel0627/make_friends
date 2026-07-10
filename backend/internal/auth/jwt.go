package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func NewRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func SignToken(userID, secret string, expireHours int, role ...string) (string, error) {
	if userID == "" || secret == "" {
		return "", errors.New("missing sign params")
	}
	jti, err := NewRefreshToken()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(time.Duration(expireHours) * time.Hour)
	claimRole := ""
	if len(role) > 0 {
		claimRole = role[0]
	}
	claims := Claims{
		UserID: userID,
		Role:   claimRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ParseToken(tokenStr, secret string) (string, error) {
	claims, err := ParseClaims(tokenStr, secret)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}

func ParseClaims(tokenStr, secret string) (*Claims, error) {
	if tokenStr == "" || secret == "" {
		return nil, errors.New("missing token params")
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	if claims.UserID == "" {
		return nil, errors.New("empty user id in token")
	}
	if claims.ID == "" {
		return nil, errors.New("empty jti in token")
	}
	return claims, nil
}
