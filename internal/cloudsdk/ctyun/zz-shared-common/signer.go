package common

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/tools/security"
)

type signer struct {
	accessKeyId     string
	secretAccessKey string
}

func (s *signer) Sign(req *http.Request) error {
	now := time.Now()
	eopDate := now.Format("20060102T150405Z")
	eopReqId := security.RandomString(32)

	queryStr := ""
	if req.URL != nil {
		queryStr = req.URL.Query().Encode()
	}

	payloadStr := ""
	if req.Method != http.MethodGet && req.Body != nil {
		payloadb, err := io.ReadAll(req.Body)
		if err != nil {
			return err
		}

		payloadStr = string(payloadb)
		req.Body = io.NopCloser(bytes.NewReader(payloadb))
	}
	payloadHash := sha256.Sum256([]byte(payloadStr))
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	var h hash.Hash
	h = hmac.New(sha256.New, []byte(s.secretAccessKey))
	h.Write([]byte(eopDate))
	kTime := h.Sum(nil)
	h = hmac.New(sha256.New, kTime)
	h.Write([]byte(s.accessKeyId))
	kAk := h.Sum(nil)
	h = hmac.New(sha256.New, kAk)
	h.Write([]byte(now.Format("20060102")))
	kDate := h.Sum(nil)

	stringToSign := fmt.Sprintf("ctyun-eop-request-id:%s\neop-date:%s\n\n%s\n%s", eopReqId, eopDate, queryStr, payloadHashHex)

	h = hmac.New(sha256.New, kDate)
	h.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	eopAuthorization := fmt.Sprintf("%s Headers=ctyun-eop-request-id;eop-date Signature=%s", s.accessKeyId, signature)

	req.Header.Set("ctyun-eop-request-id", eopReqId)
	req.Header.Set("eop-date", eopDate)
	req.Header.Set("eop-authorization", eopAuthorization)

	return nil
}
