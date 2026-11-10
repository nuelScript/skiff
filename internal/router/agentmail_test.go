package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nuelScript/skiff/internal/hooksig"
)

const multipartMail = "From: =?UTF-8?Q?Ad=C3=A1?= <ada@example.com>\r\n" +
	"Subject: =?UTF-8?Q?Invoice_=E2=82=AC40?=\r\n" +
	"Message-Id: <m1@example.com>\r\n" +
	"Content-Type: multipart/alternative; boundary=b\r\n\r\n" +
	"--b\r\nContent-Type: text/html\r\n\r\n<p>html</p>\r\n" +
	"--b\r\nContent-Type: text/plain\r\n\r\nPlease pay by Friday.\r\n" +
	"--b--\r\n"

func TestSummarizeEmail(t *testing.T) {
	e := summarizeEmail("bot@example.com", []byte(multipartMail))
	if e.From != "Adá <ada@example.com>" || e.Subject != "Invoice €40" || e.MessageID != "<m1@example.com>" || !strings.Contains(e.Text, "Please pay by Friday.") || e.To != "bot@example.com" {
		t.Errorf("%+v", e)
	}
	plain := summarizeEmail("bot@example.com", []byte("Subject: hi\r\n\r\n"+strings.Repeat("x", agentMailText+10)))
	if !plain.Truncated || len(plain.Text) != agentMailText {
		t.Errorf("long text: %d %v", len(plain.Text), plain.Truncated)
	}
}

func TestDeliverToAgentSignsTheEvent(t *testing.T) {
	var got agentEmail
	var sigOK bool
	var path, source string
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sigOK = hooksig.Check(r.Header, "s3cret", body, time.Now()) == nil
		path, source = r.URL.Path, r.Header.Get("X-Skiff-Source")
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer panel.Close()
	rt := &Router{Panel: strings.TrimPrefix(panel.URL, "http://")}
	if err := rt.deliverToAgent(context.Background(), "agent:bot:s3cret", "bot@example.com", []byte(multipartMail)); err != nil {
		t.Fatal(err)
	}
	if !sigOK || path != "/api/hooks/agents/bot" || source != "email" || got.Subject != "Invoice €40" {
		t.Errorf("sig %v path %q source %q event %+v", sigOK, path, source, got)
	}
	if err := rt.deliverToAgent(context.Background(), "agent:bot", "x@y", nil); err == nil {
		t.Error("a destination without a secret was used")
	}
}

// Text that JSON spells out at six times its size, and an endless subject, still fit the agent's inbox instead of bouncing for good.
func TestAgentEmailFitsTheInbox(t *testing.T) {
	raw := "From: a@example.com\r\nSubject: " + strings.Repeat("s", 100_000) + "\r\n\r\n" + strings.Repeat("\x01é", 30_000)
	body := encodeEmail(summarizeEmail("bot@agents.example.com", []byte(raw)))
	if len(body) > agentMailEvent {
		t.Fatalf("event is %d bytes", len(body))
	}
	var e agentEmail
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatal(err)
	}
	if !e.Truncated || len(e.Subject) > agentMailHeader || !utf8.ValidString(e.Text) || e.Text == "" {
		t.Errorf("subject %d bytes, text %d bytes, truncated %v", len(e.Subject), len(e.Text), e.Truncated)
	}
}
