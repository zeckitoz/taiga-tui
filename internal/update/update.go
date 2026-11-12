// Package update finds out whether a newer release of taiga-tui exists. It
// only looks; there is no self-update.
package update

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// LatestReleaseURL is the GitHub API endpoint of the latest release. GitHub
// leaves out pre-releases and drafts there.
const LatestReleaseURL = "https://api.github.com/repos/zeckitoz/taiga-tui/releases/latest"

// DefaultTimeout is how long the check waits for GitHub.
const DefaultTimeout = 5 * time.Second

// Interval is the time between two checks.
const Interval = 24 * time.Hour

// Checker asks GitHub for the latest release. The request is unauthenticated.
type Checker struct {
	// URL defaults to LatestReleaseURL.
	URL string
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// Timeout defaults to DefaultTimeout.
	Timeout time.Duration
}

func (c *Checker) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// Latest returns the tag of the latest release, for example "v0.4.0".
func (c *Checker) Latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	url := c.URL
	if url == "" {
		url = LatestReleaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck // nothing sensible to do about a failed close
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release: status %d", resp.StatusCode)
	}
	var rel struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}
	if rel.Prerelease || rel.Draft {
		return "", errors.New("latest release: is a pre-release or draft")
	}
	if _, ok := parse(rel.Tag); !ok {
		return "", fmt.Errorf("latest release: unusable tag %q", rel.Tag)
	}
	return rel.Tag, nil
}

// Due reports whether a check is due, given the time of the last one. A last
// check in the future (the clock went back) is due too.
func Due(last, now time.Time) bool {
	d := now.Sub(last)
	return d < 0 || d >= Interval
}

// Newer reports whether found is a newer release than current. It is false
// if either is no release version, which covers "dev", "(devel)" and Go
// pseudo-versions of untagged builds, and a pre-release found is never newer.
func Newer(current, found string) bool {
	cur, ok := parse(current)
	if !ok || cur.pseudo {
		return false
	}
	f, ok := parse(found)
	if !ok || f.pre != "" {
		return false
	}
	return cur.less(f)
}

type semver struct {
	core   [3]int
	pre    string
	pseudo bool
}

var (
	semverRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
	pseudoRe = regexp.MustCompile(`(^|\.)\d{8,}-`)
)

func parse(v string) (semver, bool) {
	m := semverRe.FindStringSubmatch(v)
	if m == nil {
		return semver{}, false
	}
	var s semver
	for i := range s.core {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return semver{}, false
		}
		s.core[i] = n
	}
	s.pre = m[4]
	s.pseudo = pseudoRe.MatchString(s.pre)
	return s, true
}

func (a semver) less(b semver) bool {
	if c := slices.Compare(a.core[:], b.core[:]); c != 0 {
		return c < 0
	}
	switch {
	case a.pre == b.pre, a.pre == "":
		return false
	case b.pre == "":
		return true
	}
	return comparePre(a.pre, b.pre) < 0
}

// comparePre orders pre-release strings by the SemVer rules.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil && an != bn:
			return cmp.Compare(an, bn)
		case aerr == nil && berr != nil:
			return -1
		case aerr != nil && berr == nil:
			return 1
		case aerr != nil && berr != nil && as[i] != bs[i]:
			return strings.Compare(as[i], bs[i])
		}
	}
	return cmp.Compare(len(as), len(bs))
}
