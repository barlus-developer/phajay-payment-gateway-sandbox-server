// Package domain defines sandbox business entities independently of HTTP and SQL.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

var (
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("operation conflicts with current state")
	ErrExpired      = errors.New("payment has expired")
)

type State string

const (
	Waiting   State = "WAITING"
	Completed State = "PAYMENT_COMPLETED"
	Failed    State = "FAILED"
	Expired   State = "EXPIRED"
	Canceled  State = "CANCELED"
)

func CanTransition(from, to State) bool {
	return from == Waiting && (to == Completed || to == Failed || to == Expired || to == Canceled)
}

// AmountMinor converts an exact JSON decimal to LAK minor units (two decimal places).
func AmountMinor(raw string) (int64, error) {
	if len(raw) > 64 || len(raw) == 0 || !json.Valid([]byte(raw)) || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return 0, ErrInvalid
	}
	if at := strings.IndexAny(raw, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(raw[at+1:])
		if err != nil || exponent < -18 || exponent > 18 {
			return 0, ErrInvalid
		}
	}
	amount, ok := new(big.Rat).SetString(raw)
	if !ok || amount.Sign() <= 0 {
		return 0, ErrInvalid
	}
	amount.Mul(amount, big.NewRat(100, 1))
	if !amount.IsInt() || !amount.Num().IsInt64() {
		return 0, ErrInvalid
	}
	minor := amount.Num().Int64()
	if minor > 1_000_000_000_000_000 {
		return 0, ErrInvalid
	}
	return minor, nil
}
func AmountNumber(minor int64) json.Number {
	return json.Number(fmt.Sprintf("%d.%02d", minor/100, minor%100))
}

type Decimal string

func (d *Decimal) UnmarshalJSON(raw []byte) error {
	if _, err := AmountMinor(string(raw)); err != nil {
		return ErrInvalid
	}
	*d = Decimal(raw)
	return nil
}

type PaymentInput struct {
	OrderNo     string  `json:"orderNo"`
	Amount      Decimal `json:"amount"`
	Description string  `json:"description"`
	Tag1        string  `json:"tag1"`
	Tag2        string  `json:"tag2"`
	Tag3        string  `json:"tag3"`
}
type Developer struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"createdAt"`
}
type Session struct {
	ID          string `json:"id"`
	DeveloperID string `json:"-"`
	TokenHash   string `json:"-"`
	CSRFHash    string `json:"-"`
	CreatedAt   int64  `json:"createdAt"`
	ExpiresAt   int64  `json:"expiresAt"`
	Current     bool   `json:"current"`
}
type Project struct {
	ID                 string `json:"id"`
	DeveloperID        string `json:"-"`
	Name               string `json:"name"`
	MerchantName       string `json:"merchantName"`
	Description        string `json:"description"`
	PaymentTTL         int64  `json:"paymentTTL"`
	WebhookMaxAttempts int    `json:"webhookMaxAttempts"`
	CreatedAt          int64  `json:"createdAt"`
}
type APIKey struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Hash       string `json:"-"`
	CreatedAt  int64  `json:"createdAt"`
	LastUsedAt *int64 `json:"lastUsedAt"`
	RevokedAt  *int64 `json:"revokedAt"`
}
type SigningKey struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	Prefix     string `json:"prefix"`
	Cipher     []byte `json:"-"`
	Active     bool   `json:"active"`
	CreatedAt  int64  `json:"createdAt"`
	ValidUntil *int64 `json:"validUntil"`
	RevokedAt  *int64 `json:"revokedAt"`
}
type Endpoint struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"projectId"`
	URL       string   `json:"url"`
	Enabled   bool     `json:"enabled"`
	Events    []string `json:"events"`
	CreatedAt int64    `json:"createdAt"`
}
type Transaction struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	OrderNo      string `json:"orderNo"`
	Kind         string `json:"kind"`
	Bank         string `json:"bank"`
	MerchantName string `json:"merchantName"`
	AmountMinor  int64  `json:"amountMinor"`
	Currency     string `json:"currency"`
	Description  string `json:"description"`
	Status       State  `json:"status"`
	CreatedAt    int64  `json:"createdAt"`
	ExpiresAt    int64  `json:"expiresAt"`
	CompletedAt  *int64 `json:"completedAt"`
	ReceiptRef   string `json:"receiptRef"`
	PaymentURL   string `json:"paymentURL"`
	CheckoutURL  string `json:"checkoutURL"`
	TokenHash    string `json:"-"`
	TokenCipher  []byte `json:"-"`
	Tag1         string `json:"tag1"`
	Tag2         string `json:"tag2"`
	Tag3         string `json:"tag3"`
}
type TransactionEvent struct {
	ID            string `json:"id"`
	TransactionID string `json:"transactionId"`
	Status        State  `json:"status"`
	CreatedAt     int64  `json:"createdAt"`
}
type Filter struct {
	Search string
	Status string
	Bank   string
	From   int64
	To     int64
	Limit  int
	Offset int
}
type APILog struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"durationMS"`
	Error      string `json:"error"`
	CreatedAt  int64  `json:"createdAt"`
}
type OutboxEvent struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	TransactionID string `json:"transactionId"`
	Type          string `json:"type"`
	Body          []byte `json:"-"`
	CreatedAt     int64  `json:"createdAt"`
}
type Job struct {
	ID            string `json:"id"`
	EventID       string `json:"eventId"`
	EndpointID    string `json:"endpointId"`
	ProjectID     string `json:"projectId"`
	URL           string `json:"url"`
	Body          []byte `json:"-"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	MaxAttempts   int    `json:"maxAttempts"`
	NextAttemptAt int64  `json:"nextAttemptAt"`
	LeaseUntil    int64  `json:"-"`
	CreatedAt     int64  `json:"createdAt"`
}
type Delivery struct {
	ID         string `json:"id"`
	JobID      string `json:"jobId"`
	EventID    string `json:"eventId"`
	EndpointID string `json:"endpointId"`
	Attempt    int    `json:"attempt"`
	HTTPStatus int    `json:"httpStatus"`
	Response   string `json:"response"`
	Error      string `json:"error"`
	DurationMS int64  `json:"durationMS"`
	CreatedAt  int64  `json:"createdAt"`
}

type Activity struct {
	Day       string `json:"day"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}
type Dashboard struct {
	Total            int64      `json:"total"`
	Completed        int64      `json:"completed"`
	Pending          int64      `json:"pending"`
	Failed           int64      `json:"failed"`
	VolumeMinor      int64      `json:"volumeMinor"`
	WebhookDelivered int64      `json:"webhookDelivered"`
	WebhookFailed    int64      `json:"webhookFailed"`
	Activity         []Activity `json:"activity"`
}
