package webhook

import (
	"context"
	phajay "github.com/barlus-developer/phajay-payment-gateway"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"
)

type staticResolver []net.IPAddr

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return r, nil }
func TestSSRFPolicy(t *testing.T) {
	for _, raw := range []string{"169.254.169.254", "0.0.0.0", "224.0.0.1", "100.100.100.200", "168.63.129.16", "::ffff:169.254.169.254", "64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::", "fe80::1"} {
		for _, dev := range []bool{false, true} {
			if AllowedIP(netip.MustParseAddr(raw), dev) {
				t.Fatalf("allowed %s", raw)
			}
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "::1"} {
		if AllowedIP(netip.MustParseAddr(raw), false) || !AllowedIP(netip.MustParseAddr(raw), true) {
			t.Fatal(raw)
		}
	}
	if !AllowedIP(netip.MustParseAddr("8.8.8.8"), false) {
		t.Fatal("public IP denied")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSendSignedCallbackWithActualSDK(t *testing.T) {
	s := New(time.Second, false)
	s.resolver = staticResolver{{IP: net.ParseIP("8.8.8.8")}}
	body := []byte(`{"paymentMethod":"JDB","transactionId":"txn_test","txnAmount":12.34}`)
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		raw, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		if r.Header.Get(EventHeader) != "evt_stable" {
			t.Fatal("event ID missing")
		}
		client := phajay.New(phajay.Config{WebhookSignatureKey: "test-signing-key"}, phajay.WithSandbox())
		var callback phajay.PaymentQRWebhookCallback
		if e := client.DecodeWebhook(raw, r.Header, &callback); e != nil || callback.TransactionID != "txn_test" {
			t.Fatal(callback, e)
		}
		stamp, e := strconv.ParseInt(r.Header.Get(TimestampHeader), 10, 64)
		if e != nil || stamp < time.Now().Unix()-1 {
			t.Fatal("timestamp", e)
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(`{"token":"private"}`)), Header: make(http.Header)}, nil
	})
	code, response, e := s.Send(context.Background(), "https://merchant.test/webhook", body, "test-signing-key", "evt_stable")
	if e != nil || code != 204 || strings.Contains(response, "private") {
		t.Fatal(code, response, e)
	}
	for _, url := range []string{"http://merchant.test/hook", "https://user:password@merchant.test", "https://merchant.test/#fragment"} {
		if e := s.Validate(context.Background(), url); e == nil {
			t.Fatal("invalid destination", url)
		}
	}
}
func TestRejectMixedDNSAnswers(t *testing.T) {
	s := New(1, false)
	s.resolver = staticResolver{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}
	if e := s.Validate(context.Background(), "https://callback.example/test"); e == nil {
		t.Fatal("mixed public/private destination accepted")
	}
}
func TestSanitize(t *testing.T) {
	got := Sanitize([]byte(`{"token":"private-value","nested":{"password":"private-password"},"result":"sk_sandbox_hidden"}`))
	if strings.Contains(got, "private") || strings.Contains(got, "sk_sandbox_") {
		t.Fatal(got)
	}
}
func TestSignatureVector(t *testing.T) {
	if got := Sign(1700000000, []byte(`{"amount":100}`), "signing-key"); got != "v1=91b6fb8cac6f35bf5b1234f49fdcad62ee625615202b5af35e8fb4e8b30add5f" {
		t.Fatal(got)
	}
}
