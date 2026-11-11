package taigarec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Recorder is an http.RoundTripper that keeps the response bodies of the
// requests sent through it in memory, so that Save can anonymise them before
// anything is written. Responses are passed on unchanged.
type Recorder struct {
	base http.RoundTripper

	mu       sync.Mutex
	captured []capturedResponse
	fixtures []Fixture
}

type capturedResponse struct {
	status int
	body   []byte
}

// NewRecorder returns a Recorder sending requests through base.
func NewRecorder(base http.RoundTripper) *Recorder {
	return &Recorder{base: base}
}

// RoundTrip implements http.RoundTripper.
func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	r.mu.Lock()
	r.captured = append(r.captured, capturedResponse{status: resp.StatusCode, body: body})
	r.mu.Unlock()
	return resp, nil
}

// Record runs call, which must send exactly one request through the Recorder
// and succeed, and keeps the response as the fixture name, e.g. "me.json".
func (r *Recorder) Record(name string, call func() error) error {
	if name != filepath.Base(name) || !strings.HasSuffix(name, ".json") {
		return fmt.Errorf("fixture name %q must be a plain .json file name", name)
	}
	r.mu.Lock()
	r.captured = nil
	r.mu.Unlock()

	err := call()

	r.mu.Lock()
	defer r.mu.Unlock()
	captured := r.captured
	r.captured = nil
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w", name, err)
	case len(captured) != 1:
		return fmt.Errorf("%s: call sent %d requests, want 1", name, len(captured))
	case captured[0].status < 200 || captured[0].status > 299:
		return fmt.Errorf("%s: HTTP %d", name, captured[0].status)
	}
	for _, f := range r.fixtures {
		if f.Name == name {
			return fmt.Errorf("%s: recorded twice", name)
		}
	}
	r.fixtures = append(r.fixtures, Fixture{Name: name, Body: captured[0].body})
	return nil
}

// Save anonymises the recorded fixtures with anon and writes them to dir. It
// writes nothing if any fixture still contains an email address, a JWT or one
// of the forbidden strings, such as the password used for recording.
func (r *Recorder) Save(dir string, anon Anonymiser, forbidden ...string) error {
	r.mu.Lock()
	fixtures := r.fixtures
	r.mu.Unlock()
	if len(fixtures) == 0 {
		return errors.New("nothing recorded")
	}

	out, err := anon.Anonymise(fixtures)
	if err != nil {
		return err
	}
	var problems []error
	for _, f := range out {
		if leaks := Leaks(f.Body); len(leaks) > 0 {
			problems = append(problems, fmt.Errorf("%s still contains %d email addresses or JWTs", f.Name, len(leaks)))
		}
		for _, s := range forbidden {
			if s != "" && bytes.Contains(f.Body, []byte(s)) {
				problems = append(problems, fmt.Errorf("%s still contains a forbidden string", f.Name))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("nothing written: %w", errors.Join(problems...))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range out {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Body, 0o644); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return nil
}
