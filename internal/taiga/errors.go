package taiga

import (
	"errors"
	"fmt"
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
	// ErrUnexpected means the response could not be understood.
	ErrUnexpected = errors.New("unexpected response from server")
)

// ServerError reports an HTTP 5xx response.
type ServerError struct{ Code int }

func (e *ServerError) Error() string { return fmt.Sprintf("server error (%d)", e.Code) }
