// Package taiga is the client for the Taiga REST API. It is the only package
// that knows Taiga's JSON structure, and it maps every failure to a typed error.
package taiga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// Tokens are the JWTs Taiga hands out on login and refresh.
type Tokens struct {
	Access  string
	Refresh string
}

// LoginResult is the outcome of a successful login.
type LoginResult struct {
	Member domain.TeamMember
	Tokens Tokens
}

// AuthAPI is the unauthenticated part of the Taiga API.
type AuthAPI struct {
	baseURL string
	http    *http.Client
}

// NewAuthAPI returns the auth API of the instance at baseURL, which has no
// trailing slash.
func NewAuthAPI(baseURL string, opts ...Option) *AuthAPI {
	return &AuthAPI{baseURL: baseURL, http: newHTTPClient(opts)}
}

type loginRequest struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	FullNameDisplay string `json:"full_name_display"`
	AuthToken       string `json:"auth_token"`
	Refresh         string `json:"refresh"`
}

// Login logs in with a Taiga username or email address and a password.
func (a *AuthAPI) Login(ctx context.Context, username, password string) (LoginResult, error) {
	code, data, err := a.post(ctx, "/auth", loginRequest{Type: "normal", Username: username, Password: password})
	if err == nil {
		err = authStatusError(code, ErrInvalidCredentials)
	}
	if err != nil {
		return LoginResult{}, err
	}

	var lr loginResponse
	if err := json.Unmarshal(data, &lr); err != nil {
		return LoginResult{}, fmt.Errorf("%w: %w", ErrUnexpected, err)
	}
	if lr.ID == 0 || lr.Username == "" || lr.AuthToken == "" || lr.Refresh == "" {
		return LoginResult{}, fmt.Errorf("%w: login response lacks Team Member or tokens", ErrUnexpected)
	}
	name := lr.FullNameDisplay
	if name == "" {
		name = lr.Username
	}
	return LoginResult{
		Member: domain.TeamMember{ID: lr.ID, Username: lr.Username, FullName: name},
		Tokens: Tokens{Access: lr.AuthToken, Refresh: lr.Refresh},
	}, nil
}

type refreshRequest struct {
	Refresh string `json:"refresh"`
}

type refreshResponse struct {
	AuthToken string `json:"auth_token"`
	Refresh   string `json:"refresh"`
}

// Refresh trades a refresh token for a new access token and a new refresh
// token. Taiga rotates refresh tokens: the one passed in is invalid afterwards.
// A rejected refresh token yields ErrSessionExpired.
func (a *AuthAPI) Refresh(ctx context.Context, refresh string) (Tokens, error) {
	code, data, err := a.post(ctx, "/auth/refresh", refreshRequest{Refresh: refresh})
	if err == nil {
		err = authStatusError(code, ErrSessionExpired)
	}
	if err != nil {
		return Tokens{}, err
	}

	var rr refreshResponse
	if err := json.Unmarshal(data, &rr); err != nil {
		return Tokens{}, fmt.Errorf("%w: %w", ErrUnexpected, err)
	}
	if rr.AuthToken == "" || rr.Refresh == "" {
		return Tokens{}, fmt.Errorf("%w: refresh response lacks tokens", ErrUnexpected)
	}
	return Tokens{Access: rr.AuthToken, Refresh: rr.Refresh}, nil
}

// authStatusError maps the status code of an auth call to a typed error;
// rejected is the error for a 400 or 401, with which Taiga rejects the
// credentials or token sent.
func authStatusError(code int, rejected error) error {
	switch {
	case code == http.StatusOK:
		return nil
	case code == http.StatusUnauthorized, code == http.StatusBadRequest:
		return rejected
	case code >= 500:
		return &ServerError{Code: code}
	}
	return fmt.Errorf("%w: HTTP %d", ErrUnexpected, code)
}

// post sends body as JSON to the API path and returns the status code and the
// response body.
func (a *AuthAPI) post(ctx context.Context, path string, body any) (int, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/v1"+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return 0, nil, transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, transportError(err)
	}
	return resp.StatusCode, data, nil
}

// transportError maps an error from sending a request or reading its response
// to ErrTimeout or ErrUnreachable. A canceled context is passed through.
func transportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return fmt.Errorf("%w: %w", ErrUnreachable, err)
}
