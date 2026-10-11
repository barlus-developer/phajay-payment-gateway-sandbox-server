package phajay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrSandboxWebhookSignatureInvalid indicates a missing, malformed, or invalid
// custom sandbox signature. This protocol is not Phajay's production protocol.
var ErrSandboxWebhookSignatureInvalid = errors.New("phajay: sandbox webhook signature is invalid")

// ErrSandboxWebhookTimestamp indicates a timestamp outside the inclusive
// five-minute tolerance in either direction.
var ErrSandboxWebhookTimestamp = errors.New("phajay: sandbox webhook timestamp is outside tolerance")

func verifySandboxSignature(body []byte, headers http.Header, key string, now time.Time) error {
	invalid := ErrSandboxWebhookSignatureInvalid
	for _, name := range []string{"X-Phajay-Sandbox-Timestamp", "X-Phajay-Sandbox-Signature", "X-Phajay-Sandbox-Event-ID"} {
		if len(headers.Values(name)) != 1 {
			return invalid
		}
	}
	stamp := headers.Get("X-Phajay-Sandbox-Timestamp")
	signature := headers.Get("X-Phajay-Sandbox-Signature")
	event := headers.Get("X-Phajay-Sandbox-Event-ID")
	if len(body) > 1<<20 || len(stamp) == 0 || len(stamp) > 12 || len(event) == 0 || len(event) > 128 || strings.TrimSpace(event) != event {
		return invalid
	}
	for _, c := range stamp {
		if c < '0' || c > '9' {
			return invalid
		}
	}
	seconds, e := strconv.ParseInt(stamp, 10, 64)
	if e != nil {
		return invalid
	}
	if seconds < now.Unix()-300 || seconds > now.Unix()+300 {
		return ErrSandboxWebhookTimestamp
	}
	if len(signature) != 67 || !strings.HasPrefix(signature, "v1=") || signature != strings.ToLower(signature) {
		return invalid
	}
	actual, e := hex.DecodeString(signature[3:])
	if e != nil {
		return invalid
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(stamp))
	mac.Write([]byte("."))
	mac.Write(body)
	if !hmac.Equal(actual, mac.Sum(nil)) {
		return invalid
	}
	return nil
}
