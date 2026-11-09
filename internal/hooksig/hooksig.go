// Package hooksig signs and checks the webhooks Skiff takes. The signature is an HMAC-SHA256 of "<unix time>.<body>", so a captured request stops working after a few minutes.
package hooksig

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

const (
	SignatureHeader = "X-Skiff-Signature"
	TimestampHeader = "X-Skiff-Timestamp"
	// MaxAge is how far a request's time may be from the receiver's clock.
	MaxAge = 5 * time.Minute
)

var (
	ErrMismatch = errors.New("the signature doesn't match")
	ErrStale    = errors.New("the request's " + TimestampHeader + " is missing or more than five minutes from now")
)

// Sign is the signature header's value for body sent at unix time ts.
func Sign(secret string, ts int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Set signs a request about to be sent with body.
func Set(h http.Header, secret string, body []byte) {
	ts := time.Now().Unix()
	h.Set(TimestampHeader, strconv.FormatInt(ts, 10))
	h.Set(SignatureHeader, Sign(secret, ts, body))
}

// Check verifies a request's signature and that it was signed within MaxAge of now.
func Check(h http.Header, secret string, body []byte, now time.Time) error {
	ts, err := strconv.ParseInt(strings.TrimSpace(h.Get(TimestampHeader)), 10, 64)
	if err != nil || now.Sub(time.Unix(ts, 0)).Abs() > MaxAge {
		return ErrStale
	}
	if secret == "" || !hmac.Equal([]byte(h.Get(SignatureHeader)), []byte(Sign(secret, ts, body))) {
		return ErrMismatch
	}
	return nil
}
