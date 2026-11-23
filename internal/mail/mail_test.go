package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
)

func TestMimeMessageHasTextAndHTML(t *testing.T) {
	raw := mimeMessage("Bosho <me@gmail.com>", Message{To: "owner@example.com", Subject: "Set a new Bosho password — বসো",
		Text: "Hello Rahim,\nlink: https://x/reset?token=abc", HTML: "<p>Hello <b>Rahim</b></p>"})
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); subj != "Set a new Bosho password — বসো" {
		t.Errorf("subject = %q", subj)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	r := multipart.NewReader(msg.Body, params["boundary"])
	var got []string
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
		got = append(got, p.Header.Get("Content-Type")+": "+string(b))
	}
	if len(got) != 2 || !strings.Contains(got[0], "token=abc") || !strings.Contains(got[1], "<b>Rahim</b>") {
		t.Fatalf("parts = %q", got)
	}
}

func TestNewPicksTheSender(t *testing.T) {
	if _, ok := New(Settings{}).(Log); !ok {
		t.Error("nothing set should log")
	}
	s, ok := New(Settings{SMTPHost: "smtp.gmail.com", SMTPUser: "me@gmail.com", ResendKey: "x"}).(*SMTP)
	if !ok || s.Port != "587" || s.From != "\"Bosho\" <me@gmail.com>" {
		t.Errorf("SMTP settings: %+v", s)
	}
	for from, want := range map[string]string{
		"Bosho":                  "\"Bosho\" <me@gmail.com>",
		"":                       "\"Bosho\" <me@gmail.com>",
		"Bosho <me@gmail.com>":   "Bosho <me@gmail.com>",
		`"Bosho <me@gmail.com>"`: "Bosho <me@gmail.com>",
		"\"My Shop\"":            "\"My Shop\" <me@gmail.com>",
	} {
		s := New(Settings{SMTPHost: "smtp.gmail.com", SMTPUser: "me@gmail.com", From: from}).(*SMTP)
		if s.From != want {
			t.Errorf("EMAIL_FROM %q: from = %q, want %q", from, s.From, want)
		}
	}
	if b, ok := New(Settings{BrevoKey: "k", SMTPHost: "smtp.gmail.com", SMTPUser: "me@gmail.com", From: "Bosho"}).(*Brevo); !ok || b.From != "\"Bosho\" <me@gmail.com>" {
		t.Errorf("Brevo key should win and keep the sender: %+v", b)
	}
	if _, ok := New(Settings{ResendKey: "x"}).(*Resend); !ok {
		t.Error("Resend key alone should use Resend")
	}
}

func TestBrevoSendsTheMessage(t *testing.T) {
	var got map[string]any
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("api-key")
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	b := &Brevo{Key: "secret", From: "Bosho <me@gmail.com>", Client: srv.Client(), URL: srv.URL}
	if err := b.Send(context.Background(), Message{To: "owner@example.com", Subject: "Hi", HTML: "<p>x</p>", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	sender, _ := got["sender"].(map[string]any)
	if key != "secret" || sender["email"] != "me@gmail.com" || sender["name"] != "Bosho" || got["subject"] != "Hi" {
		t.Fatalf("request: key=%q body=%v", key, got)
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"sender not valid"}`, http.StatusBadRequest)
	}))
	defer fail.Close()
	b.URL = fail.URL
	if err := b.Send(context.Background(), Message{To: "o@example.com"}); err == nil || !strings.Contains(err.Error(), "sender not valid") {
		t.Fatalf("error = %v", err)
	}
}
