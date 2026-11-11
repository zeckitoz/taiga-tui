package taigarec_test

import (
	"strings"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/taiga/taigarec"
)

const (
	accessJWT  = "eyJhbGciOiJIUzI1NiJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIn0.c2ln"
	refreshJWT = "eyJhbGciOiJIUzI1NiJ9.eyJ0b2tlbl90eXBlIjoicmVmcmVzaCJ9.c2ln"
)

func anonymise(t *testing.T, a taigarec.Anonymiser, fixtures ...taigarec.Fixture) map[string]string {
	t.Helper()
	out, err := a.Anonymise(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range out {
		got[f.Name] = string(f.Body)
	}
	return got
}

func TestAnonymiseReplacesNamesEmailsTokensAndPhotos(t *testing.T) {
	login := taigarec.Fixture{Name: "login.json", Body: []byte(`{
		"id": 1234, "username": "jdoe", "full_name": "Jane Doe", "full_name_display": "Jane Doe",
		"email": "jane.doe@uni.example", "bio": "Studies at the Uni",
		"description": "About Jane", "description_html": "<p>About Jane</p>",
		"photo": "https://taiga.example/media/user/jdoe.png", "big_photo": "https://taiga.example/media/user/jdoe-big.png",
		"gravatar_id": "3f2b0c", "uuid": "0f1e2d3c", "is_active": true, "kanban_order": 17000000000000001,
		"auth_token": "` + accessJWT + `", "refresh": "` + refreshJWT + `"}`)}

	got := anonymise(t, taigarec.Anonymiser{}, login)

	want := `{
  "auth_token": "redacted-access-token-1",
  "big_photo": "https://photos.example.invalid/redacted.png",
  "bio": "redacted",
  "description": "redacted",
  "description_html": "redacted",
  "email": "redacted",
  "full_name": "Member 1",
  "full_name_display": "Member 1",
  "gravatar_id": "redacted",
  "id": 1234,
  "is_active": true,
  "kanban_order": 17000000000000001,
  "photo": "https://photos.example.invalid/redacted.png",
  "refresh": "redacted-refresh-token-1",
  "username": "member-1",
  "uuid": "redacted"
}
`
	if got["login.json"] != want {
		t.Errorf("got\n%s\nwant\n%s", got["login.json"], want)
	}
}

func TestAnonymiseUsesOnePseudonymPerPersonAcrossFixturesAndInsideText(t *testing.T) {
	// The projects list comes first but only names people inside slugs and
	// subjects; who they are is learned from a later fixture.
	projects := taigarec.Fixture{Name: "projects.json", Body: []byte(`[
		{"id": 7, "name": "Secret Thesis", "slug": "jdoe-secret-thesis"},
		{"id": 9, "name": "Ask Max Muster", "slug": "mmuster-misc"}]`)}
	project := taigarec.Fixture{Name: "project_by_slug.json", Body: []byte(`{
		"id": 7, "name": "Secret Thesis", "slug": "jdoe-secret-thesis",
		"members": [
			{"id": 2, "username": "mmuster", "full_name": "Max Muster", "full_name_display": "Max Muster"},
			{"id": 1, "username": "jdoe", "full_name": "", "full_name_display": "jdoe"}]}`)}
	a := taigarec.Anonymiser{Replace: map[string]string{
		"Secret Thesis":      "Test Project",
		"jdoe-secret-thesis": "test-project",
	}}

	got := anonymise(t, a, projects, project)

	wantProjects := `[
  {
    "id": 7,
    "name": "Test Project",
    "slug": "test-project"
  },
  {
    "id": 9,
    "name": "Ask Member 1",
    "slug": "member-1-misc"
  }
]
`
	wantProject := `{
  "id": 7,
  "members": [
    {
      "full_name": "Member 1",
      "full_name_display": "Member 1",
      "id": 2,
      "username": "member-1"
    },
    {
      "full_name": "",
      "full_name_display": "Member 2",
      "id": 1,
      "username": "member-2"
    }
  ],
  "name": "Test Project",
  "slug": "test-project"
}
`
	if got["projects.json"] != wantProjects {
		t.Errorf("projects.json got\n%s\nwant\n%s", got["projects.json"], wantProjects)
	}
	if got["project_by_slug.json"] != wantProject {
		t.Errorf("project_by_slug.json got\n%s\nwant\n%s", got["project_by_slug.json"], wantProject)
	}
}

func TestAnonymiseRedactsEmailAddressesAndJWTsInAnyString(t *testing.T) {
	story := taigarec.Fixture{Name: "userstories.json", Body: []byte(`[{
		"subject": "Mail jane.doe@uni.example about it",
		"tags": [["Bearer ` + accessJWT + `", null]]}]`)}

	got := anonymise(t, taigarec.Anonymiser{}, story)

	want := `[
  {
    "subject": "Mail redacted about it",
    "tags": [
      [
        "Bearer redacted-token",
        null
      ]
    ]
  }
]
`
	if got["userstories.json"] != want {
		t.Errorf("got\n%s\nwant\n%s", got["userstories.json"], want)
	}
}

func TestAnonymiseRejectsBodiesThatAreNotJSON(t *testing.T) {
	_, err := taigarec.Anonymiser{}.Anonymise([]taigarec.Fixture{{Name: "me.json", Body: []byte("<html>Bad Gateway</html>")}})

	if err == nil || !strings.Contains(err.Error(), "me.json") {
		t.Errorf("got %v, want an error naming me.json", err)
	}
}
