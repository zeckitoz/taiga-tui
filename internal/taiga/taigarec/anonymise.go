package taigarec

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
// photos and other media URLs, free-text profiles and descriptions) are
// replaced by key. Every Team Member gets one pseudonym, numbered by their ID
// so that a new recording keeps the numbers. Their usernames, names and parts
// of names are also replaced inside every other string and object key, in
// any case, e.g. in project slugs, which start with the owner's username.
// Email addresses and JWTs are replaced wherever they appear.
type Anonymiser struct {
	// Replace maps further strings to their replacements, e.g. the name and
	// slug of the recorded project. They are replaced in any case, also
	// inside words.
	Replace map[string]string
}

const redactedURL = "https://media.example.invalid/redacted.png"

// redactedKeys are the keys whose string values are replaced as a whole.
var redactedKeys = map[string]bool{
	"email": true, "bio": true, "gravatar_id": true, "uuid": true,
	"description": true, "description_html": true,
}

// nameKeys are the keys holding a Team Member's full name.
var nameKeys = []string{"full_name", "full_name_display"}

// minNamePart is the shortest part of a name replaced on its own; shorter
// parts such as "Li" or "de" would replace ordinary words.
const minNamePart = 3

func usernamePseudonym(n int) string { return fmt.Sprintf("member-%d", n) }
func namePseudonym(n int) string     { return fmt.Sprintf("Member %d", n) }

// Anonymise returns the fixtures with personal data replaced, as indented
// JSON with sorted keys. The output depends only on the input.
func (a Anonymiser) Anonymise(fixtures []Fixture) ([]Fixture, error) {
	docs := make([]any, len(fixtures))
	for i, f := range fixtures {
		dec := json.NewDecoder(bytes.NewReader(f.Body))
		dec.UseNumber() // keeps large IDs and orders exact
		if err := dec.Decode(&docs[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
	}

	people := newDirectory()
	for _, doc := range docs {
		people.learn(doc)
	}
	s := &scrubber{
		people: people.number(),
		tokens: map[string]map[string]int{},
	}
	s.text = newTextReplacer(s.people, a.Replace)

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

// directory collects the Team Members found in the fixtures.
type directory struct {
	byUsername map[string]*member
	// looseNames are names seen only in objects without a username.
	looseNames map[string]bool
}

type member struct {
	username string
	id       int64 // 0 if no object with the username carried an ID
	names    []string
}

func newDirectory() *directory {
	return &directory{byUsername: map[string]*member{}, looseNames: map[string]bool{}}
}

func (d *directory) learn(doc any) {
	switch v := doc.(type) {
	case map[string]any:
		username, _ := v["username"].(string)
		for _, k := range nameKeys {
			name, _ := v[k].(string)
			switch {
			case name == "":
			case username != "":
				d.member(username).addName(name)
			default:
				d.looseNames[name] = true
			}
		}
		if username != "" {
			m := d.member(username)
			if id, ok := v["id"].(json.Number); ok && m.id == 0 {
				m.id, _ = strconv.ParseInt(id.String(), 10, 64)
			}
		}
		for _, k := range sortedKeys(v) {
			d.learn(v[k])
		}
	case []any:
		for _, e := range v {
			d.learn(e)
		}
	}
}

func (d *directory) member(username string) *member {
	m := d.byUsername[username]
	if m == nil {
		m = &member{username: username}
		d.byUsername[username] = m
	}
	return m
}

func (m *member) addName(name string) {
	if !slices.Contains(m.names, name) {
		m.names = append(m.names, name)
	}
}

// people maps every username and name to the number of its Team Member.
type people struct {
	usernames map[string]int
	names     map[string]int
}

// number numbers the Team Members by ID, those without an ID after them by
// username, and names never seen with a username last, by name.
func (d *directory) number() people {
	members := slices.Collect(maps.Values(d.byUsername))
	slices.SortFunc(members, func(a, b *member) int {
		if (a.id == 0) != (b.id == 0) {
			if a.id == 0 {
				return 1
			}
			return -1
		}
		return cmp.Or(cmp.Compare(a.id, b.id), strings.Compare(a.username, b.username))
	})
	p := people{usernames: map[string]int{}, names: map[string]int{}}
	for i, m := range members {
		p.usernames[m.username] = i + 1
		for _, name := range m.names {
			if _, ok := p.names[name]; !ok {
				p.names[name] = i + 1
			}
		}
	}
	n := len(members)
	for _, name := range slices.Sorted(maps.Keys(d.looseNames)) {
		if _, ok := p.names[name]; !ok {
			n++
			p.names[name] = n
		}
	}
	return p
}

// scrubber rewrites decoded JSON.
type scrubber struct {
	people people
	text   *textReplacer
	tokens map[string]map[string]int // key -> original token -> number
}

// value returns v with personal data replaced; key is the object key v
// belongs to, or "" for array elements and the document itself.
func (s *scrubber) value(key string, v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for _, k := range sortedKeys(v) {
			out[s.text.replace(k)] = s.value(k, v[k])
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
	switch {
	case v == "":
		return v
	case key == "username":
		return usernamePseudonym(s.people.usernames[v])
	case slices.Contains(nameKeys, key):
		return namePseudonym(s.people.names[v])
	case redactedKeys[key]:
		return "redacted"
	case key == "photo" || key == "big_photo" || strings.HasSuffix(key, "_url"):
		return redactedURL
	case key == "auth_token":
		return fmt.Sprintf("redacted-access-token-%d", s.token(key, v))
	case key == "refresh":
		return fmt.Sprintf("redacted-refresh-token-%d", s.token(key, v))
	}
	return s.text.replace(v)
}

// token numbers the tokens of each key in order of appearance.
func (s *scrubber) token(key, v string) int {
	m := s.tokens[key]
	if m == nil {
		m = map[string]int{}
		s.tokens[key] = m
	}
	if _, ok := m[v]; !ok {
		m[v] = len(m) + 1
	}
	return m[v]
}

// textReplacer replaces personal values inside free text.
type textReplacer struct {
	pattern *regexp.Regexp       // nil if there is nothing to replace
	entries map[string]textEntry // by lower-case original
}

type textEntry struct {
	replacement string
	// wholeWord restricts the replacement to matches not inside a longer
	// word, so that a username "ab" leaves "#AB12CD" alone.
	wholeWord bool
}

// newTextReplacer replaces, in this priority: the extra strings, usernames,
// full names, and parts of names.
func newTextReplacer(p people, extra map[string]string) *textReplacer {
	entries := map[string]textEntry{}
	add := func(original string, e textEntry) {
		key := strings.ToLower(original)
		if _, ok := entries[key]; !ok && key != "" {
			entries[key] = e
		}
	}
	for _, original := range slices.Sorted(maps.Keys(extra)) {
		add(original, textEntry{replacement: extra[original]})
	}
	for _, username := range slices.Sorted(maps.Keys(p.usernames)) {
		add(username, textEntry{replacement: usernamePseudonym(p.usernames[username]), wholeWord: true})
	}
	names := slices.Sorted(maps.Keys(p.names))
	for _, name := range names {
		add(name, textEntry{replacement: namePseudonym(p.names[name]), wholeWord: true})
	}
	for _, name := range names {
		for _, part := range strings.FieldsFunc(name, func(r rune) bool { return !isWordRune(r) }) {
			if utf8.RuneCountInString(part) >= minNamePart {
				add(part, textEntry{replacement: namePseudonym(p.names[name]), wholeWord: true})
			}
		}
	}
	if len(entries) == 0 {
		return &textReplacer{}
	}

	// Longest first, so that a slug wins over the username it contains.
	originals := slices.Collect(maps.Keys(entries))
	slices.SortFunc(originals, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	quoted := make([]string, len(originals))
	for i, o := range originals {
		quoted[i] = regexp.QuoteMeta(o)
	}
	return &textReplacer{
		pattern: regexp.MustCompile("(?i)" + strings.Join(quoted, "|")),
		entries: entries,
	}
}

func (t *textReplacer) replace(s string) string {
	if t.pattern != nil {
		var b strings.Builder
		last := 0
		for _, loc := range t.pattern.FindAllStringIndex(s, -1) {
			match := s[loc[0]:loc[1]]
			e, ok := t.entries[strings.ToLower(match)]
			if !ok {
				// Case folding matched a spelling ToLower does not produce.
				e = textEntry{replacement: "redacted"}
			}
			if e.wholeWord && !wordBoundary(s, loc[0], loc[1]) {
				continue
			}
			b.WriteString(s[last:loc[0]])
			b.WriteString(e.replacement)
			last = loc[1]
		}
		b.WriteString(s[last:])
		s = b.String()
	}
	s = emailPattern.ReplaceAllLiteralString(s, "redacted")
	return jwtPattern.ReplaceAllLiteralString(s, "redacted-token")
}

// wordBoundary reports whether s[start:end] is not part of a longer word.
func wordBoundary(s string, start, end int) bool {
	before, _ := utf8.DecodeLastRuneInString(s[:start])
	after, _ := utf8.DecodeRuneInString(s[end:])
	return (start == 0 || !isWordRune(before)) && (end == len(s) || !isWordRune(after))
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
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
