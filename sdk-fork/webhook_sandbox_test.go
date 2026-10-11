package phajay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sandboxHeaders(body []byte, key string, stamp int64) http.Header {
	raw := strconv.FormatInt(stamp, 10)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(raw + "."))
	mac.Write(body)
	h := make(http.Header)
	h.Set("X-Phajay-Sandbox-Timestamp", raw)
	h.Set("X-Phajay-Sandbox-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))
	h.Set("X-Phajay-Sandbox-Event-ID", "evt_unique")
	return h
}
func TestSandboxSignatureVerification(t *testing.T) {
	now := time.Unix(1700000000, 0)
	body := []byte(`{"transactionId":"tx","paymentMethod":"JDB","txnAmount":12.34}`)
	key := "signing-key"
	tests := []struct {
		name    string
		mutate  func(http.Header)
		payload []byte
		key     string
		stamp   int64
		want    error
	}{
		{"valid", nil, body, key, now.Unix(), nil},
		{"modified", nil, append(append([]byte{}, body...), ' '), key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"wrong key", nil, body, "wrong", now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"missing", func(h http.Header) { h.Del("X-Phajay-Sandbox-Signature") }, body, key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"incorrect", func(h http.Header) { h.Set("X-Phajay-Sandbox-Signature", "v1="+strings.Repeat("0", 64)) }, body, key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"uppercase", func(h http.Header) {
			h.Set("X-Phajay-Sandbox-Signature", strings.ToUpper(h.Get("X-Phajay-Sandbox-Signature")))
		}, body, key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"duplicate", func(h http.Header) { h.Add("X-Phajay-Sandbox-Timestamp", h.Get("X-Phajay-Sandbox-Timestamp")) }, body, key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"missing event", func(h http.Header) { h.Del("X-Phajay-Sandbox-Event-ID") }, body, key, now.Unix(), ErrSandboxWebhookSignatureInvalid},
		{"expired", nil, body, key, now.Unix() - 301, ErrSandboxWebhookTimestamp},
		{"future", nil, body, key, now.Unix() + 301, ErrSandboxWebhookTimestamp},
		{"boundary past", nil, body, key, now.Unix() - 300, nil},
		{"boundary future", nil, body, key, now.Unix() + 300, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sandboxHeaders(body, key, tt.stamp)
			if tt.mutate != nil {
				tt.mutate(h)
			}
			if e := verifySandboxSignature(tt.payload, h, tt.key, now); !errors.Is(e, tt.want) {
				t.Fatalf("got %v want %v", e, tt.want)
			}
		})
	}
}
func TestSandboxDecodeAndProductionPolicy(t *testing.T) {
	body := []byte(`{"paymentMethod":"JDB","transactionId":"tx","txnAmount":12.34}`)
	h := sandboxHeaders(body, "new-key", time.Now().Unix())
	client := New(Config{WebhookSignatureKey: "new-key"}, WithSandbox())
	var callback PaymentQRWebhookCallback
	if e := client.DecodeWebhook(body, h, &callback); e != nil || callback.TransactionID != "tx" {
		t.Fatalf("decode: %+v %v", callback, e)
	}
	old := New(Config{WebhookSignatureKey: "old-key"}, WithSandbox())
	if e := old.VerifyWebhookSignature(body, h); e == nil {
		t.Fatal("old key accepted new signature")
	}
	if e := New(Config{WebhookSignatureKey: "new-key"}).VerifyWebhookSignature(body, h); !errors.Is(e, ErrWebhookSignatureVerificationUnavailable) {
		t.Fatalf("production changed: %v", e)
	}
	if e := New(Config{}, WithSandbox()).VerifyWebhookSignature(body, h); !errors.Is(e, ErrWebhookSignatureKeyMissing) {
		t.Fatalf("missing key: %v", e)
	}
	callback.TransactionID = "untouched"
	if e := client.DecodeWebhook([]byte(`{}`), h, &callback); e == nil || callback.TransactionID != "untouched" {
		t.Fatal("invalid signature decoded payload")
	}
}
