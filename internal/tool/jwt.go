package tool

import (
	"crypto/rand"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"gogs.io/gogs/internal/conf"
	"time"
)

type Subject int

const (
	SubjectActiveAccount Subject = 1
	SubjectActiveEmail   Subject = 2
	SubjectForgetPasswd  Subject = 3
)

var secretKey = make([]byte, 32)

func init() {
	if _, err := rand.Read(secretKey); err != nil {
		panic(err)
	}
}

type Claims struct {
	Audience  string  `json:"aud,omitempty"`
	ExpiresAt int64   `json:"exp,omitempty"`
	Id        int64   `json:"jti,omitempty"`
	Email     string  `json:"email,omitempty"`
	IssuedAt  int64   `json:"iat,omitempty"`
	Issuer    string  `json:"iss,omitempty"`
	NotBefore int64   `json:"nbf,omitempty"`
	Subject   Subject `json:"sub,omitempty"`
}

func (c *Claims) Valid() error {
	now := time.Now()

	if now.After(time.Unix(c.ExpiresAt, 0)) {
		return fmt.Errorf("error")
	}

	if now.Before(time.Unix(c.NotBefore, 0)) {
		return fmt.Errorf("error")
	}

	if now.Before(time.Unix(c.IssuedAt, 0)) {
		return fmt.Errorf("error")
	}

	if c.Audience != c.Email {
		return fmt.Errorf("error")
	}

	return nil
}

func NewClaims(id int64, email string, subject Subject) *Claims {
	now := time.Now()
	return &Claims{
		Audience:  email,
		ExpiresAt: now.Add(time.Duration(conf.Auth.ActivateCodeLives) * time.Minute).Unix(),
		Id:        id,
		Email:     email,
		IssuedAt:  now.Unix(),
		Issuer:    conf.Server.ExternalURL,
		NotBefore: now.Unix(),
		Subject:   subject,
	}
}

func (c *Claims) ToToken() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	//使用指定的secret签名并获得完成的编码后的字符串token

	return token.SignedString(secretKey)
}

func ParseToken(t string) (*Claims, error) {
	//解析token
	token, err := jwt.ParseWithClaims(t, &Claims{}, func(token *jwt.Token) (i interface{}, err error) {
		return secretKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && claims != nil && token.Valid {
		return claims, nil
	} else if err := claims.Valid(); err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && claims != nil && token.Valid {
		if err := claims.Valid(); err != nil {
			return nil, err
		}
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}
