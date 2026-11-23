// Package mail sends Bosho's emails (verify email, reset password) through an SMTP server such as
// Gmail, or through Resend, or prints them to the log when neither is set (enough for local development).
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To, Subject, HTML, Text string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Settings choose how mail is sent: SMTP when SMTPHost is set, else Resend when ResendKey is set,
// else the log.
type Settings struct {
	BrevoKey                                   string
	SMTPHost, SMTPPort, SMTPUser, SMTPPassword string
	ResendKey                                  string
	From                                       string
}

// New picks the sender: Brevo, then SMTP, then Resend, then the log. Brevo and Resend send over
// HTTPS, which works on hosts that block SMTP ports (Render's free plan does).
func New(s Settings) Sender {
	// EMAIL_FROM may be a full "Name <address>" or just a name ("Bosho"): with only a name, or
	// nothing, the address is SMTP_USER (Gmail sends from the logged-in account anyway).
	// Quotes around the whole value (EMAIL_FROM="Bosho <me@gmail.com>" typed into a dashboard) are dropped.
	from := strings.TrimSpace(s.From)
	if len(from) >= 2 && (from[0] == '"' || from[0] == '\'') && from[len(from)-1] == from[0] && strings.Contains(from, "<") {
		from = strings.TrimSpace(from[1 : len(from)-1])
	}
	if _, err := mail.ParseAddress(from); err != nil {
		name := strings.Trim(from, `"'`)
		if name == "" {
			name = "Bosho"
		}
		from = (&mail.Address{Name: name, Address: s.SMTPUser}).String()
	}
	switch {
	case s.BrevoKey != "":
		return &Brevo{Key: s.BrevoKey, From: from, Client: &http.Client{Timeout: 10 * time.Second}}
	case s.SMTPHost != "":
		port := s.SMTPPort
		if port == "" {
			port = "587"
		}
		return &SMTP{Host: s.SMTPHost, Port: port, User: s.SMTPUser, Password: s.SMTPPassword, From: from}
	case s.ResendKey != "":
		return &Resend{Key: s.ResendKey, From: s.From, Client: &http.Client{Timeout: 10 * time.Second}}
	}
	return Log{}
}

// Log prints the message, links included, to the server log instead of sending it.
type Log struct{}

func (Log) Send(_ context.Context, m Message) error {
	slog.Info("email (not sent: no BREVO_API_KEY, SMTP_HOST or RESEND_API_KEY)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

// Resend sends through https://resend.com. From must be an address on a domain verified there,
// for example "Bosho <no-reply@bosho.food>".
type Resend struct {
	Key, From string
	Client    *http.Client
}

func (s *Resend) Send(ctx context.Context, m Message) error {
	body, _ := json.Marshal(map[string]any{"from": s.From, "to": []string{m.To}, "subject": m.Subject, "html": m.HTML, "text": m.Text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("resend: status %d: %s", resp.StatusCode, msg)
	}
	return nil
}

// SMTP sends through an SMTP server, for example Gmail: host smtp.gmail.com, port 587, the Gmail
// address as user and an app password (Google account -> Security -> 2-Step Verification ->
// App passwords). The connection is upgraded to TLS before logging in.
type SMTP struct {
	Host, Port, User, Password string
	From                       string // "Bosho <you@gmail.com>"; Gmail sends from the logged-in address
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return errors.New("smtp: the server does not offer STARTTLS")
	}
	if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
		return err
	}
	if err := c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
		return fmt.Errorf("smtp login: %w", err)
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("EMAIL_FROM: %w", err)
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(mimeMessage(from.String(), m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// mimeMessage builds a plain-text + HTML email.
func mimeMessage(from string, m Message) []byte {
	boundary := fmt.Sprintf("bosho-%d", time.Now().UnixNano())
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n",
		from, m.To, mime.QEncoding.Encode("utf-8", m.Subject), time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	for _, part := range []struct{ typ, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", boundary, part.typ)
		enc := base64.StdEncoding.EncodeToString([]byte(part.body))
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

// Brevo sends through https://www.brevo.com over HTTPS. The From address must be a sender verified
// in Brevo (Senders, domains & dedicated IPs -> Senders); a single Gmail address is enough.
type Brevo struct {
	Key, From string
	Client    *http.Client
	URL       string // empty: Brevo's API (tests point it elsewhere)
}

func (s *Brevo) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil || from.Address == "" {
		return fmt.Errorf("EMAIL_FROM needs an address for Brevo, for example \"Bosho <you@gmail.com>\"")
	}
	body, _ := json.Marshal(map[string]any{
		"sender":      map[string]string{"name": from.Name, "email": from.Address},
		"to":          []map[string]string{{"email": m.To}},
		"subject":     m.Subject,
		"htmlContent": m.HTML,
		"textContent": m.Text,
	})
	url := s.URL
	if url == "" {
		url = "https://api.brevo.com/v3/smtp/email"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", s.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("brevo: status %d: %s", resp.StatusCode, msg)
	}
	return nil
}
