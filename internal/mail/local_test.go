package mail

import (
	"context"
	"testing"
	"time"
)

func TestLocalDestinationsSkipSMTP(t *testing.T) {
	var got []string
	f := &Forwarder{Local: func(_ context.Context, dst, rcpt string, _ []byte) error {
		got = append(got, dst+"<-"+rcpt)
		return nil
	}}
	env := Envelope{From: "a@b.com", Data: []byte("x"), Recipients: []Recipient{
		{Addr: "bot@example.com", Destinations: []string{"agent:bot:secret", "agent:bot:secret"}},
	}}
	if err := f.Forward(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "agent:bot:secret<-bot@example.com" {
		t.Errorf("local deliveries: %v", got)
	}
	if jobs := f.plan(env, time.Now()); len(jobs) != 0 {
		t.Errorf("an agent destination was planned for SMTP: %+v", jobs)
	}
}
