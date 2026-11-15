package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// OneDrive holds the Microsoft account a OneDrive target uses. Kiestoon signs in with the OAuth 2.0
// device code flow ("go to microsoft.com/devicelogin and enter this code"), so no redirect address
// has to be reachable from the internet. The portal keeps the refresh token and hands rclone a fresh
// access token for every run.
type OneDrive struct {
	// Tenant is "common", "organizations", "consumers" or the directory (tenant) ID.
	Tenant       string `json:"tenant"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
	SecretSet    bool   `json:"secretSet"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Connected    bool   `json:"connected"`
	DriveID      string `json:"driveId"`
	DriveType    string `json:"driveType"` // personal | business | documentLibrary
	Account      string `json:"account"`
	// AccessToken and Expiry only travel from the portal to the helper.
	AccessToken string    `json:"accessToken,omitempty"`
	Expiry      time.Time `json:"expiry,omitempty"`
}

var (
	reTenant = regexp.MustCompile(`^(common|organizations|consumers|[0-9a-fA-F-]{36}|[A-Za-z0-9.-]{1,200}\.[A-Za-z]{2,})$`)
	reGUID   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reDrive  = regexp.MustCompile(`^[A-Za-z0-9!_.-]{1,200}$`)
)

// Validate checks the account settings.
func (o *OneDrive) Validate() error {
	o.Tenant = strings.TrimSpace(o.Tenant)
	if o.Tenant == "" {
		o.Tenant = "common"
	}
	o.ClientID = strings.TrimSpace(o.ClientID)
	if !reTenant.MatchString(o.Tenant) {
		return errors.New("tenant")
	}
	if !reGUID.MatchString(o.ClientID) {
		return errors.New("clientId")
	}
	if o.DriveID != "" && !reDrive.MatchString(o.DriveID) {
		return errors.New("driveId")
	}
	switch o.DriveType {
	case "", "personal", "business", "documentLibrary":
	default:
		return errors.New("driveType")
	}
	return nil
}

const graphScope = "Files.ReadWrite offline_access User.Read"

func (o OneDrive) authority() string {
	return "https://login.microsoftonline.com/" + url.PathEscape(o.Tenant) + "/oauth2/v2.0"
}

// DeviceCode is the start of a sign-in: show UserCode and VerificationURI to the administrator.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ErrPending means the administrator has not finished signing in yet.
var ErrPending = errors.New("authorization_pending")

func postForm(ctx context.Context, client *http.Client, u string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return resp.StatusCode, fmt.Errorf("HTTP %d: unexpected answer", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// StartDeviceLogin asks Microsoft for a device code.
func (o OneDrive) StartDeviceLogin(ctx context.Context, client *http.Client) (*DeviceCode, error) {
	var resp struct {
		DeviceCode
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	form := url.Values{"client_id": {o.ClientID}, "scope": {graphScope}}
	code, err := postForm(ctx, client, o.authority()+"/devicecode", form, &resp)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK || resp.DeviceCode.DeviceCode == "" {
		return nil, tokenErr(tokenResponse{Error: resp.Error, ErrorDescription: resp.ErrorDescription}, code)
	}
	return &resp.DeviceCode, nil
}

func (o *OneDrive) apply(t tokenResponse) {
	o.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		o.RefreshToken = t.RefreshToken
	}
	o.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UTC()
}

func tokenErr(t tokenResponse, code int) error {
	msg := t.ErrorDescription
	if i := strings.Index(msg, "\r\n"); i > 0 {
		msg = msg[:i]
	}
	return fmt.Errorf("token: %s (HTTP %d) %s", t.Error, code, msg)
}

// PollDeviceLogin checks whether the sign-in of a device code is done. It returns ErrPending while
// the administrator is still busy.
func (o *OneDrive) PollDeviceLogin(ctx context.Context, client *http.Client, deviceCode string) error {
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {o.ClientID}, "device_code": {deviceCode}}
	if o.ClientSecret != "" {
		form.Set("client_secret", o.ClientSecret)
	}
	var t tokenResponse
	code, err := postForm(ctx, client, o.authority()+"/token", form, &t)
	if err != nil {
		return err
	}
	if t.Error == "authorization_pending" || t.Error == "slow_down" {
		return ErrPending
	}
	if code != http.StatusOK || t.AccessToken == "" {
		return tokenErr(t, code)
	}
	o.apply(t)
	return o.discoverDrive(ctx, client)
}

// Refresh gets a new access token (and usually a new refresh token, which must be stored).
func (o *OneDrive) Refresh(ctx context.Context, client *http.Client) error {
	if o.RefreshToken == "" {
		return errors.New("onedrive_not_connected")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {o.ClientID}, "refresh_token": {o.RefreshToken}, "scope": {graphScope}}
	if o.ClientSecret != "" {
		form.Set("client_secret", o.ClientSecret)
	}
	var t tokenResponse
	code, err := postForm(ctx, client, o.authority()+"/token", form, &t)
	if err != nil {
		return err
	}
	if code != http.StatusOK || t.AccessToken == "" {
		return tokenErr(t, code)
	}
	o.apply(t)
	return nil
}

// GraphURL is the Microsoft Graph endpoint (a variable for tests).
var GraphURL = "https://graph.microsoft.com/v1.0"

// discoverDrive looks up the OneDrive of the signed-in account.
func (o *OneDrive) discoverDrive(ctx context.Context, client *http.Client) error {
	var d struct {
		ID        string `json:"id"`
		DriveType string `json:"driveType"`
		Owner     struct {
			User struct {
				DisplayName string `json:"displayName"`
				Email       string `json:"email"`
			} `json:"user"`
		} `json:"owner"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GraphURL+"/me/drive", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.AccessToken)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("onedrive: HTTP %d (does the account have a OneDrive?)", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&d); err != nil {
		return err
	}
	o.DriveID, o.DriveType = d.ID, d.DriveType
	o.Account = d.Owner.User.Email
	if o.Account == "" {
		o.Account = d.Owner.User.DisplayName
	}
	o.Connected = true
	return nil
}

// rcloneToken is the token JSON rclone expects in its "token" option.
func (o OneDrive) rcloneToken() string {
	b, _ := json.Marshal(map[string]string{"access_token": o.AccessToken, "token_type": "Bearer", "refresh_token": o.RefreshToken,
		"expiry": o.Expiry.Format(time.RFC3339)})
	return string(b)
}
