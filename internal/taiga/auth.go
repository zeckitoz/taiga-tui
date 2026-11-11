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
	"time"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// DefaultTimeout is the time a request may take, including reading the response.
const DefaultTimeout = 15 * time.Second

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

// Option configures an AuthAPI.
type Option func(*AuthAPI)

// WithTimeout replaces DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(a *AuthAPI) { a.http.Timeout = d }
}

// NewAuthAPI returns the auth API of the instance at baseURL, which has no
// trailing slash.
func NewAuthAPI(baseURL string, opts ...Option) *AuthAPI {
	a := &AuthAPI{baseURL: baseURL, http: &http.Client{Timeout: DefaultTimeout}}
	for _, opt := range opts {
		opt(a)
	}
	return a
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
	body, err := json.Marshal(loginRequest{Type: "normal", Username: username, Password: password})
	if err != nil {
		return LoginResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/v1/auth", bytes.NewReader(body))
	if err != nil {
		return LoginResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return LoginResult{}, transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return LoginResult{}, transportError(err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusBadRequest:
		return LoginResult{}, ErrInvalidCredentials
	case resp.StatusCode >= 500:
		return LoginResult{}, &ServerError{Code: resp.StatusCode}
	case resp.StatusCode != http.StatusOK:
		return LoginResult{}, fmt.Errorf("%w: HTTP %d", ErrUnexpected, resp.StatusCode)
	}

	var lr loginResponse
	if err := json.Unmarshal(data, &lr); err != nil {
		return LoginResult{}, fmt.Errorf("%w: %w", ErrUnexpected, err)
	}
	if lr.ID == 0 || lr.Username == "" || lr.AuthToken == "" || lr.Refresh == "" {
		return LoginResult{}, fmt.Errorf("%w: login response lacks user or tokens", ErrUnexpected)
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
