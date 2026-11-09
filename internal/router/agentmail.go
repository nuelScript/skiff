package router

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
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
	"time"

	skiffmail "github.com/nuelScript/skiff/internal/mail"
)

// agentMailText is how much of a message's text an agent's event carries, within the webhook's 64 KB.
const agentMailText = 40 << 10

// agentEmail is what an agent gets for a message: who, what, and the readable text, not the raw message with its attachments.
type agentEmail struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Date      string `json:"date,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

// summarizeEmail reads a message into what an agent needs, preferring its plain-text part.
func summarizeEmail(rcpt string, data []byte) agentEmail {
	out := agentEmail{To: rcpt}
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		out.Text = string(data)
		return capText(out)
	}
	dec := new(mime.WordDecoder)
	decode := func(v string) string {
		if s, err := dec.DecodeHeader(v); err == nil {
			return s
		}
		return v
	}
	out.From, out.Subject = decode(msg.Header.Get("From")), decode(msg.Header.Get("Subject"))
	out.Date, out.MessageID = msg.Header.Get("Date"), msg.Header.Get("Message-Id")
	out.Text = plainText(msg.Header.Get("Content-Type"), msg.Body)
	return capText(out)
}

func plainText(contentType string, body io.Reader) string {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(media, "multipart/") {
		b, _ := io.ReadAll(io.LimitReader(body, 1<<20))
		return string(b)
	}
	var html string
	mr := multipart.NewReader(body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		pt, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch {
		case pt == "text/plain":
			b, _ := io.ReadAll(io.LimitReader(part, 1<<20))
			return string(b)
		case strings.HasPrefix(pt, "multipart/"):
			if s := plainText(part.Header.Get("Content-Type"), part); s != "" {
				return s
			}
		case pt == "text/html" && html == "":
			b, _ := io.ReadAll(io.LimitReader(part, 1<<20))
			html = string(b)
		}
	}
	return html
}

func capText(e agentEmail) agentEmail {
	if len(e.Text) > agentMailText {
		e.Text, e.Truncated = e.Text[:agentMailText], true
	}
	return e
}

// deliverToAgent posts a message to the agent's webhook on the panel, signed with the agent's secret, as an email event.
func (rt *Router) deliverToAgent(ctx context.Context, dst, rcpt string, data []byte) error {
	parts := strings.SplitN(dst, ":", 3)
	if len(parts) != 3 || parts[0] != "agent" || parts[2] == "" {
		return errors.New("not an agent destination")
	}
	up := rt.panelUpstream()
	if up == "" {
		return &skiffmail.Error{Temporary: true, Msg: "the panel isn't reachable"}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(summarizeEmail(rcpt, data))
	body := buf.Bytes()
	mac := hmac.New(sha256.New, []byte(parts[2]))
	mac.Write(body)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+strings.TrimPrefix(up, "http://")+"/api/hooks/agents/"+parts[1], bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Skiff-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Skiff-Source", "email")
	req.Header.Set("X-Skiff-Event", "email")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &skiffmail.Error{Temporary: true, Msg: err.Error()}
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return &skiffmail.Error{Temporary: true, Msg: fmt.Sprintf("the agent's inbox answered %d", resp.StatusCode)}
	}
	return &skiffmail.Error{Msg: fmt.Sprintf("the agent's inbox answered %d", resp.StatusCode)}
}
