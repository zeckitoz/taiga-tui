package taigarec

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Fixture is one recorded response body, saved as Name in the testdata
// directory.
type Fixture struct {
	Name string
	Body []byte
}

// Anonymiser replaces personal data in recorded responses.
//
// Fields that hold personal data (usernames, names, email addresses, tokens,
// photos) are replaced by key. The usernames and names found anywhere in the
// fixtures, plus the Replace keys, are also replaced inside every other
// string, e.g. in project slugs, which start with the owner's username.
type Anonymiser struct {
	// Replace maps further strings to their replacements, e.g. the name and
	// slug of the recorded project.
	Replace map[string]string
}

const redactedPhoto = "https://photos.example.invalid/redacted.png"

// Anonymise returns the fixtures with personal data replaced, as indented
// JSON with sorted keys.
func (a Anonymiser) Anonymise(fixtures []Fixture) ([]Fixture, error) {
	docs := make([]any, len(fixtures))
	for i, f := range fixtures {
		dec := json.NewDecoder(bytes.NewReader(f.Body))
		dec.UseNumber() // keeps large IDs and orders exact
		if err := dec.Decode(&docs[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
	}

	s := newScrubber()
	for _, doc := range docs {
		s.learn(doc)
	}
	s.text = s.textReplacer(a.Replace)
	out := make([]Fixture, len(fixtures))
	for i, doc := range docs {
		body, err := encode(s.value("", doc))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fixtures[i].Name, err)
		}
		out[i] = Fixture{Name: fixtures[i].Name, Body: body}
	}
	return out, nil
}

// scrubber rewrites decoded JSON. Each distinct original value gets a
// numbered pseudonym in order of first appearance, so the output is
// deterministic and the same person or token reads the same everywhere.
type scrubber struct {
	numbers map[string]map[string]int // kind -> original -> number
	counts  map[string]int            // kind -> highest number handed out
	// usernames holds the original usernames; other "person" values are names.
	usernames map[string]bool
	// text replaces personal values inside free text; built after learning.
	text *strings.Replacer
}

func newScrubber() *scrubber {
	return &scrubber{numbers: map[string]map[string]int{}, counts: map[string]int{}, usernames: map[string]bool{}}
}

func (s *scrubber) number(kind, original string) int {
	m := s.numbers[kind]
	if m == nil {
		m = map[string]int{}
		s.numbers[kind] = m
	}
	n, ok := m[original]
	if !ok {
		s.counts[kind]++
		n = s.counts[kind]
		m[original] = n
	}
	return n
}

// learn numbers every person in doc, in order of appearance.
func (s *scrubber) learn(doc any) {
	switch v := doc.(type) {
	case map[string]any:
		s.linkNames(v)
		for _, k := range []string{"full_name", "full_name_display"} {
			if name, _ := v[k].(string); name != "" {
				s.number("person", name)
			}
		}
		for _, k := range sortedKeys(v) {
			s.learn(v[k])
		}
	case []any:
		for _, e := range v {
			s.learn(e)
		}
	}
}

// textReplacer replaces the given strings and every learned username and
// name, longest first so that a slug wins over the username it contains.
func (s *scrubber) textReplacer(extra map[string]string) *strings.Replacer {
	pairs := map[string]string{}
	for original, n := range s.numbers["person"] {
		if s.usernames[original] {
			pairs[original] = fmt.Sprintf("member-%d", n)
		} else {
			pairs[original] = fmt.Sprintf("Member %d", n)
		}
	}
	maps.Copy(pairs, extra)
	originals := slices.Collect(maps.Keys(pairs))
	slices.SortFunc(originals, func(a, b string) int {
		if c := cmp.Compare(len(b), len(a)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	var oldnew []string
	for _, o := range originals {
		if o != "" {
			oldnew = append(oldnew, o, pairs[o])
		}
	}
	return strings.NewReplacer(oldnew...)
}

// linkNames gives the full names of an object with a username the number of
// that username, so a Team Member's username and name share one pseudonym.
func (s *scrubber) linkNames(obj map[string]any) {
	username, _ := obj["username"].(string)
	if username == "" {
		return
	}
	n := s.number("person", username)
	s.usernames[username] = true
	for _, k := range []string{"full_name", "full_name_display"} {
		if name, _ := obj[k].(string); name != "" {
			if _, seen := s.numbers["person"][name]; !seen {
				s.numbers["person"][name] = n
			}
		}
	}
}

// value returns v with personal data replaced; key is the object key v
// belongs to, or "" for array elements and the document itself.
func (s *scrubber) value(key string, v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for _, k := range sortedKeys(v) {
			out[k] = s.value(k, v[k])
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = s.value("", e)
		}
		return out
	case string:
		return s.field(key, v)
	default:
		return v
	}
}

// field replaces the string value of the object key.
func (s *scrubber) field(key, v string) string {
	if v == "" {
		return v
	}
	switch key {
	case "username":
		return fmt.Sprintf("member-%d", s.number("person", v))
	case "full_name", "full_name_display":
		return fmt.Sprintf("Member %d", s.number("person", v))
	case "email", "bio", "gravatar_id", "uuid", "description", "description_html":
		return "redacted"
	case "photo", "big_photo":
		return redactedPhoto
	case "auth_token":
		return fmt.Sprintf("redacted-access-token-%d", s.number(key, v))
	case "refresh":
		return fmt.Sprintf("redacted-refresh-token-%d", s.number(key, v))
	}
	v = s.text.Replace(v)
	v = emailPattern.ReplaceAllLiteralString(v, "redacted")
	return jwtPattern.ReplaceAllLiteralString(v, "redacted-token")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
