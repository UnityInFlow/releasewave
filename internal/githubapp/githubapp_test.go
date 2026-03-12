package githubapp

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// writeTestKey generates a 2048-bit RSA key, writes it to a temp file in
// PKCS#1 PEM format, and returns the file path and the private key itself.
// The caller is responsible for removing the file.
func writeTestKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}

	f, err := os.CreateTemp(t.TempDir(), "test-key-*.pem")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := pem.Encode(f, block); err != nil {
		f.Close()
		t.Fatalf("encode PEM: %v", err)
	}
	f.Close()

	return f.Name(), key
}

// ---------------------------------------------------------------------------
// app.go tests
// ---------------------------------------------------------------------------

func TestNew_ZeroAppID(t *testing.T) {
	_, err := New(Config{AppID: 0, PrivateKeyPath: "/tmp/some.pem"})
	if err == nil {
		t.Fatal("expected error for zero AppID, got nil")
	}
}

func TestNew_EmptyPrivateKeyPath(t *testing.T) {
	_, err := New(Config{AppID: 42, PrivateKeyPath: ""})
	if err == nil {
		t.Fatal("expected error for empty PrivateKeyPath, got nil")
	}
}

func TestNew_NonExistentKeyFile(t *testing.T) {
	_, err := New(Config{AppID: 42, PrivateKeyPath: "/tmp/does-not-exist-ever.pem"})
	if err == nil {
		t.Fatal("expected error for non-existent key file, got nil")
	}
}

func TestNew_ValidKey(t *testing.T) {
	keyPath, _ := writeTestKey(t)

	app, err := New(Config{AppID: 123, PrivateKeyPath: keyPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil App")
	}
	if app.config.AppID != 123 {
		t.Fatalf("expected AppID 123, got %d", app.config.AppID)
	}
}

func TestGenerateJWT_ValidToken(t *testing.T) {
	keyPath, key := writeTestKey(t)

	app, err := New(Config{AppID: 456, PrivateKeyPath: keyPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tokenString, err := app.generateJWT()
	if err != nil {
		t.Fatalf("generateJWT: %v", err)
	}
	if tokenString == "" {
		t.Fatal("expected non-empty JWT")
	}

	// Parse and verify the token with the public key.
	parsed, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse JWT: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("JWT is not valid")
	}

	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok {
		t.Fatal("unexpected claims type")
	}

	// iss must equal the string representation of AppID.
	if claims.Issuer != strconv.FormatInt(456, 10) {
		t.Fatalf("expected issuer '456', got %q", claims.Issuer)
	}

	now := time.Now()

	// iat should be approximately now - 60s.
	if claims.IssuedAt == nil {
		t.Fatal("IssuedAt is nil")
	}
	iatDiff := math.Abs(now.Add(-60 * time.Second).Sub(claims.IssuedAt.Time).Seconds())
	if iatDiff > 5 {
		t.Fatalf("IssuedAt drift too large: %.1fs", iatDiff)
	}

	// exp should be approximately now + 10m.
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	expDiff := math.Abs(now.Add(10 * time.Minute).Sub(claims.ExpiresAt.Time).Seconds())
	if expDiff > 5 {
		t.Fatalf("ExpiresAt drift too large: %.1fs", expDiff)
	}
}

// ---------------------------------------------------------------------------
// webhook.go tests
// ---------------------------------------------------------------------------

const testSecret = "test-webhook-secret"

func TestVerifySignature_Valid(t *testing.T) {
	payload := []byte(`{"action":"created"}`)
	sig := FormatSignature(payload, testSecret)

	if !VerifySignature(payload, sig, testSecret) {
		t.Fatal("expected valid signature to verify")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	payload := []byte(`{"action":"created"}`)
	sig := "sha256=0000000000000000000000000000000000000000000000000000000000000000"

	if VerifySignature(payload, sig, testSecret) {
		t.Fatal("expected invalid signature to fail verification")
	}
}

func TestVerifySignature_EmptyAndShort(t *testing.T) {
	payload := []byte(`{"action":"created"}`)

	cases := []struct {
		name string
		sig  string
	}{
		{"empty", ""},
		{"too short", "sha256"},
		{"no prefix", "abcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if VerifySignature(payload, tc.sig, testSecret) {
				t.Fatalf("expected VerifySignature to return false for signature %q", tc.sig)
			}
		})
	}
}

func TestFormatSignature_Verifiable(t *testing.T) {
	payload := []byte(`hello world`)
	sig := FormatSignature(payload, testSecret)

	if len(sig) < 8 {
		t.Fatalf("signature too short: %q", sig)
	}
	if sig[:7] != "sha256=" {
		t.Fatalf("signature missing sha256= prefix: %q", sig)
	}
	if !VerifySignature(payload, sig, testSecret) {
		t.Fatal("FormatSignature output did not verify")
	}
}

// makeReleasePayload builds a minimal GitHub release webhook payload.
func makeReleasePayload(t *testing.T) []byte {
	t.Helper()
	payload := map[string]interface{}{
		"action": "published",
		"release": map[string]interface{}{
			"tag_name": "v1.2.3",
			"html_url": "https://github.com/org/repo/releases/tag/v1.2.3",
		},
		"repository": map[string]interface{}{
			"full_name": "org/repo",
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// makeInstallationPayload builds a minimal GitHub installation webhook payload.
func makeInstallationPayload(t *testing.T) []byte {
	t.Helper()
	payload := map[string]interface{}{
		"action": "created",
		"installation": map[string]interface{}{
			"id": 99,
			"account": map[string]interface{}{
				"login": "test-org",
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestWebhookHandler_ReleaseEvent(t *testing.T) {
	handler := NewWebhookHandler(testSecret)

	var gotRepo, gotTag, gotURL string
	handler.OnRelease(func(repo, tag, url string) {
		gotRepo = repo
		gotTag = tag
		gotURL = url
	})

	body := makeReleasePayload(t)
	sig := FormatSignature(body, testSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "release")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotRepo != "org/repo" {
		t.Fatalf("expected repo 'org/repo', got %q", gotRepo)
	}
	if gotTag != "v1.2.3" {
		t.Fatalf("expected tag 'v1.2.3', got %q", gotTag)
	}
	if gotURL != "https://github.com/org/repo/releases/tag/v1.2.3" {
		t.Fatalf("expected url 'https://github.com/org/repo/releases/tag/v1.2.3', got %q", gotURL)
	}
}

func TestWebhookHandler_InstallationEvent(t *testing.T) {
	handler := NewWebhookHandler(testSecret)

	var gotID int64
	var gotAccount string
	handler.OnInstall(func(installationID int64, account string) {
		gotID = installationID
		gotAccount = account
	})

	body := makeInstallationPayload(t)
	sig := FormatSignature(body, testSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "installation")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotID != 99 {
		t.Fatalf("expected installation ID 99, got %d", gotID)
	}
	if gotAccount != "test-org" {
		t.Fatalf("expected account 'test-org', got %q", gotAccount)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	handler := NewWebhookHandler(testSecret)

	body := []byte(`{"action":"published"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadb")
	req.Header.Set("X-GitHub-Event", "release")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWebhookHandler_UnhandledEvent(t *testing.T) {
	handler := NewWebhookHandler(testSecret)

	body := []byte(`{"action":"completed"}`)
	sig := FormatSignature(body, testSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "check_run")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for unhandled event, got %d", rec.Code)
	}
}
