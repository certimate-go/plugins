package utils

import (
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/google/uuid"
)

const (
	HighMask = 0xf0
	LowMask  = 0x0f
)

var HexCodeTable = []string{
	"0", "1", "2", "3",
	"4", "5", "6", "7",
	"8", "9", "a", "b",
	"c", "d", "e", "f",
}

func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func Base64Decode(encodedString string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encodedString)
}

func GenerateRSASignature(data []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hash := md5.New()
	hash.Write(data)
	hashed := hash.Sum(nil)
	return rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.MD5, hashed)
}

func Nonce() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

func ConvertToHexString(data []byte) string {
	if data == nil {
		return ""
	}
	builder := strings.Builder{}
	for _, d := range data {
		builder.WriteString(HexCodeTable[(HighMask&d)>>4])
		builder.WriteString(HexCodeTable[LowMask&d])
	}
	return builder.String()
}

func Sha256Encode(text string) []byte {
	h := sha256.New()
	h.Write([]byte(text))
	return h.Sum(nil)
}

func HmacSha1(text string, keyStr string) []byte {
	key := []byte(keyStr)
	mac := hmac.New(sha1.New, key)
	mac.Write([]byte(text))
	return mac.Sum(nil)
}

func HmacSha256(text string, keyStr string) []byte {
	key := []byte(keyStr)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(text))
	return mac.Sum(nil)
}

func RSAPublicKeyEncrypt(publicKey *rsa.PublicKey, data []byte) ([]byte, error) {
	return rsa.EncryptPKCS1v15(nil, publicKey, data)
}
