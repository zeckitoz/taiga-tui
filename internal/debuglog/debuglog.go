// Package debuglog records the HTTP traffic of --debug in a form that is safe
// to attach to a bug report: tokens and the password never reach the log.
package debuglog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Redacted replaces every secret value in the log.
const Redacted = "[redacted]"

// secretHeaders are the headers whose values are replaced, in canonical form.
var secretHeaders = map[string]bool{
	"Authorization": true,
	"Cookie":        true,
	"Set-Cookie":    true,
}

// secretFields are the keys of JSON objects whose values are replaced.
var secretFields = map[string]bool{
	"password":   true,
	"refresh":    true,
	"auth_token": true,
}

// Transport is an http.RoundTripper that writes every request and response
// to a log and passes them on unchanged.
type Transport struct {
	base http.RoundTripper

	mu  sync.Mutex
	out io.Writer
}

// NewTransport returns a Transport sending requests through base and logging
// them to out. Writes to out are serialized.
func NewTransport(base http.RoundTripper, out io.Writer) *Transport {
	return &Transport{base: base, out: out}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		if reqBody, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}
	t.write("-> %s %s", req.Method, req.URL, headerLines(req.Header), bodyLines(reqBody))

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	took := time.Since(start).Round(time.Millisecond)
	if err != nil {
		t.write("<- %s %s failed after %s: %v", req.Method, req.URL, took, err)
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.write("<- %s %s %s after %s, reading the body failed: %v", resp.Status, req.Method, req.URL, took, err)
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	t.write("<- %s %s %s (%s)", resp.Status, req.Method, req.URL, took, headerLines(resp.Header), bodyLines(respBody))
	return resp, nil
}

// write logs one entry: a timestamped title line and the indented detail
// lines, which are the trailing []string arguments.
func (t *Transport) write(format string, args ...any) {
	var detail []string
	for len(args) > 0 {
		lines, ok := args[len(args)-1].([]string)
		if !ok {
			break
		}
		detail = append(lines, detail...)
		args = args[:len(args)-1]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	for _, l := range detail {
		b.WriteString("    " + l + "\n")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = io.WriteString(t.out, b.String())
}

func headerLines(h http.Header) []string {
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	sort.Strings(names)
	var lines []string
	for _, name := range names {
		for _, v := range h[name] {
			if secretHeaders[http.CanonicalHeaderKey(name)] {
				v = Redacted
			}
			lines = append(lines, name+": "+v)
		}
	}
	return lines
}

// bodyLines renders a body on one line. JSON bodies have their secrets
// replaced; anything else is not logged, because nothing says where a secret
// in it would be.
func bodyLines(body []byte) []string {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return []string{fmt.Sprintf("[body of %d bytes, not JSON]", len(body))}
	}
	out, err := json.Marshal(redact(v))
	if err != nil {
		return []string{fmt.Sprintf("[body of %d bytes, not logged]", len(body))}
	}
	return []string{string(out)}
}

func redact(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			if secretFields[k] {
				v[k] = Redacted
			} else {
				v[k] = redact(child)
			}
		}
	case []any:
		for i, child := range v {
			v[i] = redact(child)
		}
	}
	return v
}
