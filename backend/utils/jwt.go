package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/user/server-ops-backend/config"
	"github.com/user/server-ops-backend/models"
)

var ErrInvalidAdminSession = errors.New("invalid administrator session")

// Claims contains the singleton administrator session identity.
type Claims struct {
	AdminID        uint   `json:"admin_id"`
	Username       string `json:"username"`
	SessionVersion uint64 `json:"session_version"`
	jwt.RegisteredClaims
}

func GenerateToken(adminID uint, username string, sessionVersion uint64) (string, error) {
	cfg := config.LoadConfig()
	now := time.Now()
	claims := Claims{
		AdminID:        adminID,
		Username:       username,
		SessionVersion: sessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour * time.Duration(cfg.TokenExpiration))),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWTSecret))
}

func ParseToken(tokenString string) (*Claims, error) {
	cfg := config.LoadConfig()
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func ValidateAdminToken(tokenString string) (*Claims, *models.AdminAccount, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: token validation failed", ErrInvalidAdminSession)
	}

	admin, err := models.GetAdminAccount()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: administrator unavailable", ErrInvalidAdminSession)
	}
	if claims.AdminID != admin.ID ||
		claims.SessionVersion != admin.SessionVersion ||
		claims.Username != admin.Username {
		return nil, nil, ErrInvalidAdminSession
	}

	return claims, admin, nil
}
