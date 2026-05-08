package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpDigits = 6
	totpPeriod = 30
)

func NormalizeAuthenticatorSecret(secret string) string {
	secret = strings.ToUpper(secret)
	secret = strings.ReplaceAll(secret, " ", "")
	secret = strings.ReplaceAll(secret, "-", "")
	return secret
}

func ValidateAuthenticatorSecret(secret string) (string, error) {
	normalized := NormalizeAuthenticatorSecret(secret)
	if normalized == "" {
		return "", fmt.Errorf("authenticator secret is empty")
	}
	_, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid authenticator secret: %w", err)
	}
	return normalized, nil
}

func GenerateAuthenticatorSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

func BuildAuthenticatorURI(issuer string, account string, secret string) string {
	label := url.PathEscape(fmt.Sprintf("%s:%s", issuer, account))
	values := url.Values{}
	values.Set("secret", secret)
	values.Set("issuer", issuer)
	values.Set("algorithm", "SHA1")
	values.Set("digits", fmt.Sprintf("%d", totpDigits))
	values.Set("period", fmt.Sprintf("%d", totpPeriod))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, values.Encode())
}

func VerifyAuthenticatorCode(secret string, code string) bool {
	normalized, err := ValidateAuthenticatorSecret(secret)
	if err != nil {
		return false
	}
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	now := time.Now()
	for offset := -1; offset <= 1; offset++ {
		if generateTOTPCode(normalized, now.Add(time.Duration(offset*totpPeriod)*time.Second)) == code {
			return true
		}
	}
	return false
}

func generateTOTPCode(secret string, t time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	counter := uint64(t.Unix() / totpPeriod)
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)

	hash := hmac.New(sha1.New, key)
	_, _ = hash.Write(counterBytes[:])
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := int(binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff)
	modulo := 1
	for i := 0; i < totpDigits; i++ {
		modulo *= 10
	}
	value %= modulo
	return fmt.Sprintf("%0*d", totpDigits, value)
}
