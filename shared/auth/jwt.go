package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidSignature = errors.New("invalid token signature")
)

// Claims represents the JWT claims
type Claims struct {
	ClientID string `json:"client_id"`
	Role     string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

// TokenManager handles JWT token operations
type TokenManager struct {
	secretKey []byte
	issuer    string
}

// NewTokenManager creates a new token manager
func NewTokenManager(secretKey, issuer string) *TokenManager {
	return &TokenManager{
		secretKey: []byte(secretKey),
		issuer:    issuer,
	}
}

// GenerateToken creates a new JWT token for a client
func (tm *TokenManager) GenerateToken(clientID string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		ClientID: clientID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.issuer,
			Subject:   clientID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.secretKey)
}

// ValidateToken validates a JWT token and returns the claims
func (tm *TokenManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSignature
		}
		return tm.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshToken creates a new token with extended expiration
func (tm *TokenManager) RefreshToken(oldToken string, expiresIn time.Duration) (string, error) {
	claims, err := tm.ValidateToken(oldToken)
	if err != nil {
		// Allow refresh even if expired, but not if invalid
		if !errors.Is(err, ErrExpiredToken) {
			return "", err
		}
	}

	return tm.GenerateToken(claims.ClientID, expiresIn)
}

// GenerateRandomToken generates a random token for simple authentication
func GenerateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateClientID checks if the client ID is valid
func ValidateClientID(clientID string) bool {
	// Basic validation: non-empty and reasonable length
	return len(clientID) > 0 && len(clientID) <= 128
}
