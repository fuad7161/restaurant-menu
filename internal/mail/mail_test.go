package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
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
		"Bosho":                "\"Bosho\" <me@gmail.com>",
		"":                     "\"Bosho\" <me@gmail.com>",
		"Bosho <me@gmail.com>": "Bosho <me@gmail.com>",
		"\"My Shop\"":          "\"My Shop\" <me@gmail.com>",
	} {
		s := New(Settings{SMTPHost: "smtp.gmail.com", SMTPUser: "me@gmail.com", From: from}).(*SMTP)
		if s.From != want {
			t.Errorf("EMAIL_FROM %q: from = %q, want %q", from, s.From, want)
		}
	}
	if _, ok := New(Settings{ResendKey: "x"}).(*Resend); !ok {
		t.Error("Resend key alone should use Resend")
	}
}
