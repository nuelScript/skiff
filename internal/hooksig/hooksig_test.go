package hooksig

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	body := []byte(`{"a":1}`)
	h := http.Header{}
	Set(h, "s3cret", body)
	now := time.Now()
	if err := Check(h, "s3cret", body, now); err != nil {
		t.Fatalf("a fresh signed request: %v", err)
	}
	if err := Check(h, "other", body, now); !errors.Is(err, ErrMismatch) {
		t.Errorf("another secret: %v", err)
	}
	if err := Check(h, "s3cret", []byte(`{"a":2}`), now); !errors.Is(err, ErrMismatch) {
		t.Errorf("another body: %v", err)
	}
	if err := Check(h, "s3cret", body, now.Add(6*time.Minute)); !errors.Is(err, ErrStale) {
		t.Errorf("an old request: %v", err)
	}
	moved := h.Clone()
	moved.Set(TimestampHeader, strconv.FormatInt(now.Unix()+60, 10))
	if err := Check(moved, "s3cret", body, now); !errors.Is(err, ErrMismatch) {
		t.Errorf("a changed timestamp: %v", err)
	}
	if err := Check(http.Header{SignatureHeader: h[SignatureHeader]}, "s3cret", body, now); !errors.Is(err, ErrStale) {
		t.Errorf("no timestamp: %v", err)
	}
}
