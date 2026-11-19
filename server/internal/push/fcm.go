package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
)

// FCMSender sends to Android phones through Firebase Cloud Messaging (HTTP v1).
type FCMSender struct {
	ProjectID string
	Tokens    oauth2.TokenSource
	HTTP      *http.Client
	// BaseURL overrides https://fcm.googleapis.com (tests).
	BaseURL string
}

// NewFCMSender reads a Firebase service account file (JSON).
func NewFCMSender(serviceAccount []byte) (*FCMSender, error) {
	var sa struct {
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(serviceAccount, &sa); err != nil {
		return nil, err
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("fcm: incomplete service account file")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	cfg := &jwt.Config{
		Email:      sa.ClientEmail,
		PrivateKey: []byte(sa.PrivateKey),
		Scopes:     []string{"https://www.googleapis.com/auth/firebase.messaging"},
		TokenURL:   sa.TokenURI,
	}
	return &FCMSender{ProjectID: sa.ProjectID, Tokens: oauth2.ReuseTokenSource(nil, cfg.TokenSource(context.Background()))}, nil
}

// fcmMessage builds the message for one kind. Everything is a data message: the app decides what
// to show, so the content stays encrypted up to the phone.
func fcmMessage(m Message) map[string]any {
	data := map[string]string{"k": m.Kind}
	if m.ID != "" {
		data["id"] = m.ID
	}
	if m.Data != "" {
		data["e"] = m.Data
	}
	android := map[string]any{"priority": "HIGH", "ttl": "86400s"}
	switch m.Kind {
	case KindCall, KindCancel:
		android["ttl"] = "30s"
	}
	if m.Kind == KindVoicemail && m.ID != "" {
		android["collapse_key"] = m.ID
	}
	return map[string]any{"message": map[string]any{"token": m.Token, "data": data, "android": android}}
}

// Send delivers one notification.
func (f *FCMSender) Send(ctx context.Context, m Message) error {
	tok, err := f.Tokens.Token()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(fcmMessage(m))
	base := f.BaseURL
	if base == "" {
		base = "https://fcm.googleapis.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/projects/"+f.ProjectID+"/messages:send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	tok.SetAuthHeader(req)
	hc := f.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode == http.StatusOK {
		return nil
	}
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if res.StatusCode == http.StatusNotFound || e.Error.Status == "NOT_FOUND" {
		return ErrGone
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return ErrGone
		}
	}
	return fmt.Errorf("fcm: %d %s", res.StatusCode, e.Error.Status)
}
