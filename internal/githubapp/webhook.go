package githubapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// WebhookHandler processes GitHub App webhook events.
type WebhookHandler struct {
	secret    string
	onInstall func(installationID int64, account string)
	onRelease func(repo, tag, url string)
}

// NewWebhookHandler creates a webhook handler.
func NewWebhookHandler(secret string) *WebhookHandler {
	return &WebhookHandler{secret: secret}
}

// OnInstall registers a callback for installation events.
func (h *WebhookHandler) OnInstall(fn func(installationID int64, account string)) {
	h.onInstall = fn
}

// OnRelease registers a callback for release events.
func (h *WebhookHandler) OnRelease(fn func(repo, tag, url string)) {
	h.onRelease = fn
}

// ServeHTTP handles incoming webhook requests.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is configured.
	if h.secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, h.secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	slog.Debug("githubapp.webhook", "event", eventType)

	switch eventType {
	case "installation":
		h.handleInstallation(body)
	case "release":
		h.handleRelease(body)
	default:
		slog.Debug("githubapp.webhook.unhandled", "event", eventType)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) handleInstallation(body []byte) {
	var event struct {
		Action       string `json:"action"`
		Installation struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		slog.Error("githubapp.webhook.parse", "error", err)
		return
	}

	slog.Info("githubapp.installation",
		"action", event.Action,
		"id", event.Installation.ID,
		"account", event.Installation.Account.Login,
	)

	if h.onInstall != nil && event.Action == "created" {
		h.onInstall(event.Installation.ID, event.Installation.Account.Login)
	}
}

func (h *WebhookHandler) handleRelease(body []byte) {
	var event struct {
		Action  string `json:"action"`
		Release struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
		} `json:"release"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		slog.Error("githubapp.webhook.parse", "error", err)
		return
	}

	if event.Action != "published" {
		return
	}

	slog.Info("githubapp.release",
		"repo", event.Repository.FullName,
		"tag", event.Release.TagName,
	)

	if h.onRelease != nil {
		h.onRelease(event.Repository.FullName, event.Release.TagName, event.Release.HTMLURL)
	}
}

func verifySignature(payload []byte, signature, secret string) bool {
	if len(signature) < 8 || signature[:7] != "sha256=" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature[7:]), []byte(expected))
}

// Handler returns this webhook handler as http.Handler (it already implements it).
func (h *WebhookHandler) Handler() http.Handler {
	return h
}

// VerifySignature is exported for testing.
func VerifySignature(payload []byte, signature, secret string) bool {
	return verifySignature(payload, signature, secret)
}

// FormatSignature creates a valid signature for testing.
func FormatSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}
