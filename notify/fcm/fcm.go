// Package fcm sends push notifications through Firebase Cloud Messaging
// (HTTP v1) using a Google service account. It is intentionally self-contained:
// only the Go standard library, so the ARTEX binary keeps its minimal
// dependency footprint and does not need to fetch golang.org/x/oauth2 at build
// time.
package fcm

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	fcmScope    = "https://www.googleapis.com/auth/firebase.messaging"
	oauthToken  = "https://oauth2.googleapis.com/token"
	oauthAud    = "https://oauth2.googleapis.com/token"
	fcmEndpoint = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
)

// ErrInvalidToken reports that the target device token was rejected by FCM as
// invalid or unregistered; callers should delete it.
var ErrInvalidToken = errors.New("fcm token invalid or unregistered")

// Message is a single push to one device token.
type Message struct {
	Token string
	Title string
	Body  string
	// Data carries the deep-link payload (values must be strings).
	Data map[string]string
}

// Client sends FCM messages for one Firebase project.
type Client struct {
	projectID   string
	clientEmail string
	privateKey  *rsa.PrivateKey

	http *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

// NewFromServiceAccountJSON builds a Client from a raw service-account JSON.
func NewFromServiceAccountJSON(raw []byte) (*Client, error) {
	var sa serviceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("service account missing project_id/client_email/private_key")
	}
	key, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &Client{
		projectID:   sa.ProjectID,
		clientEmail: sa.ClientEmail,
		privateKey:  key,
		http:        &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// NewFromServiceAccountFile reads the service-account JSON from path.
func NewFromServiceAccountFile(path string) (*Client, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("empty service account path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service account: %w", err)
	}
	return NewFromServiceAccountJSON(raw)
}

func parseRSAPrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("service account private_key is not valid PEM")
	}
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("service account private_key is not RSA")
	}
	return rsaKey, nil
}

// getAccessToken returns a cached OAuth2 access token, refreshing it when needed.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.expiresAt.Add(-time.Minute)) {
		return c.accessToken, nil
	}
	assertion, err := c.jwtAssertion()
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthToken, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth token status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode oauth token: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("oauth token response missing access_token")
	}
	c.accessToken = out.AccessToken
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	c.expiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

// jwtAssertion builds the short-lived JWT used to exchange for an OAuth2 token.
func (c *Client) jwtAssertion() (string, error) {
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss":   c.clientEmail,
		"scope": fcmScope,
		"aud":   oauthAud,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Send delivers one message. It returns ErrInvalidToken when FCM reports the
// device token is no longer valid, so the caller can drop it.
func (c *Client) Send(ctx context.Context, msg Message) error {
	if msg.Token == "" {
		return errors.New("fcm message missing token")
	}
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	message := map[string]any{
		"token":        msg.Token,
		"notification": map[string]any{"title": msg.Title, "body": msg.Body},
		"android":      map[string]any{"priority": "high"},
	}
	if len(msg.Data) > 0 {
		data := make(map[string]string, len(msg.Data))
		for k, v := range msg.Data {
			data[k] = v
		}
		message["data"] = data
	}
	payload, err := json.Marshal(map[string]any{"message": message})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(fcmEndpoint, c.projectID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fcm send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrInvalidToken
	case http.StatusBadRequest:
		if strings.Contains(string(body), "UNREGISTERED") || strings.Contains(string(body), "INVALID_ARGUMENT") {
			return ErrInvalidToken
		}
	}
	return fmt.Errorf("fcm status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
