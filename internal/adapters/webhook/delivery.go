// Package webhook implements the custom sandbox signing protocol and SSRF-safe delivery.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const TimestampHeader = "X-Phajay-Sandbox-Timestamp"
const SignatureHeader = "X-Phajay-Sandbox-Signature"
const EventHeader = "X-Phajay-Sandbox-Event-ID"

func Sign(timestamp int64, body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

type resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type Sender struct {
	client     *http.Client
	resolver   resolver
	allowLocal bool
}

func New(timeout time.Duration, allowLocal bool) *Sender {
	s := &Sender{resolver: net.DefaultResolver, allowLocal: allowLocal}
	transport := &http.Transport{Proxy: nil, DialContext: s.dial, MaxIdleConns: 10, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: timeout}
	s.client = &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return s
}
func (s *Sender) Close() { s.client.CloseIdleConnections() }
func (s *Sender) Validate(ctx context.Context, raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return fmt.Errorf("invalid webhook URL")
	}
	if u.Scheme != "https" && !(s.allowLocal && u.Scheme == "http") {
		return fmt.Errorf("HTTPS is required")
	}
	port := u.Port()
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid webhook port")
		}
	}
	_, e = s.addresses(ctx, u.Hostname())
	return e
}
func (s *Sender) addresses(ctx context.Context, host string) ([]net.IPAddr, error) {
	ips, e := s.resolver.LookupIPAddr(ctx, host)
	if e != nil || len(ips) == 0 {
		return nil, fmt.Errorf("destination could not be resolved")
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok || !AllowedIP(addr, s.allowLocal) {
			return nil, fmt.Errorf("destination network is prohibited")
		}
	}
	return ips, nil
}

// AllowedIP always rejects metadata, unspecified, multicast and reserved networks.
func AllowedIP(ip netip.Addr, allowLocal bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "168.63.129.16/32", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32"} {
		if netip.MustParsePrefix(raw).Contains(ip) {
			return false
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return allowLocal
	}
	return ip.IsGlobalUnicast()
}
func (s *Sender) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := s.addresses(ctx, host)
	if e != nil {
		return nil, e
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	var failure error
	for _, ip := range ips {
		connection, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if e == nil {
			return connection, nil
		}
		failure = e
	}
	return nil, failure
}
func (s *Sender) Send(ctx context.Context, destination string, body []byte, key, event string) (int, string, error) {
	if len(body) > 1<<20 || key == "" || event == "" {
		return 0, "", errors.New("invalid delivery")
	}
	if e := s.Validate(ctx, destination); e != nil {
		return 0, "", e
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, destination, bytes.NewReader(body))
	if e != nil {
		return 0, "", e
	}
	timestamp := time.Now().Unix()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(TimestampHeader, strconv.FormatInt(timestamp, 10))
	request.Header.Set(SignatureHeader, Sign(timestamp, body, key))
	request.Header.Set(EventHeader, event)
	request.Header.Set("User-Agent", "Phajay-Sandbox/1.0")
	response, e := s.client.Do(request)
	if e != nil {
		return 0, "", errors.New("callback connection failed")
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, 4096))
	if e != nil {
		return response.StatusCode, "", errors.New("callback response could not be read")
	}
	return response.StatusCode, Sanitize(raw), nil
}

var secretPattern = regexp.MustCompile(`(?i)(sk_sandbox_|whsec_sandbox_|ses_|csrf_|pay_)[a-z0-9_-]+`)

func Sanitize(raw []byte) string {
	var value any
	if json.Unmarshal(raw, &value) == nil {
		redact(value)
		if body, e := json.Marshal(value); e == nil {
			return secretPattern.ReplaceAllString(string(body), "[REDACTED]")
		}
	}
	// Unstructured callback responses may contain arbitrary application secrets.
	// Retain structured redacted JSON; omit everything else from persistent logs.
	return "[Non-JSON response omitted]"
}
func redact(value any) {
	switch x := value.(type) {
	case map[string]any:
		for key, child := range x {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "signature") || strings.Contains(lower, "key") {
				x[key] = "[REDACTED]"
			} else {
				redact(child)
			}
		}
	case []any:
		for _, child := range x {
			redact(child)
		}
	}
}
