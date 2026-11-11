package taiga

import (
	"errors"
	"fmt"
	"net/http"
)

// Typed errors returned by the Taiga client. Callers match them with
// errors.Is or errors.AsType and map them to texts for the Team Member.
var (
	// ErrInvalidCredentials means Taiga rejected the username or password.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrUnreachable means no connection to the instance could be made.
	ErrUnreachable = errors.New("no connection")
	// ErrTimeout means the request did not finish within the request timeout.
	ErrTimeout = errors.New("timed out")
	// ErrSessionExpired means Taiga rejected the access token.
	ErrSessionExpired = errors.New("session expired")
	// ErrConflict means the item was changed by someone else since it was loaded.
	ErrConflict = errors.New("changed by someone else")
	// ErrNotFound means the requested item does not exist (any more).
	ErrNotFound = errors.New("not found")
	// ErrUnexpected means the response could not be understood.
	ErrUnexpected = errors.New("unexpected response from server")
)

// ServerError reports an HTTP 5xx response.
type ServerError struct{ Code int }

func (e *ServerError) Error() string { return fmt.Sprintf("server error (%d)", e.Code) }

// RejectedError reports that Taiga rejected the credentials or token of an
// auth call with a 400 or 401. It unwraps to ErrInvalidCredentials for a login
// and to ErrSessionExpired for a token refresh.
type RejectedError struct {
	// Code is the HTTP status code, 400 or 401.
	Code int
	// Reason is Taiga's explanation on one line, without the secrets sent, or
	// empty if Taiga gave none.
	Reason string
	// Err is ErrInvalidCredentials or ErrSessionExpired.
	Err error
}

func (e *RejectedError) Error() string {
	if e.Reason == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + e.Reason
}

func (e *RejectedError) Unwrap() error { return e.Err }

// InvalidInput reports whether Taiga rejected the input as malformed, such as
// a blank field, rather than as wrong credentials or an invalid token.
func (e *RejectedError) InvalidInput() bool { return e.Code == http.StatusBadRequest }
