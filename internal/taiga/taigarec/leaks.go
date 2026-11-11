package taigarec

import (
	"regexp"
	"slices"
)

var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*`)
	// A JWT is three base64url segments; its header always starts with {" = eyJ.
	jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
)

// Leaks returns the email addresses and JWTs found in data, in order of
// appearance. A fixture must have none.
func Leaks(data []byte) []string {
	type match struct {
		at   int
		text string
	}
	var found []match
	for _, re := range []*regexp.Regexp{emailPattern, jwtPattern} {
		for _, loc := range re.FindAllIndex(data, -1) {
			found = append(found, match{loc[0], string(data[loc[0]:loc[1]])})
		}
	}
	slices.SortFunc(found, func(a, b match) int { return a.at - b.at })
	out := make([]string, len(found))
	for i, m := range found {
		out[i] = m.text
	}
	return out
}
