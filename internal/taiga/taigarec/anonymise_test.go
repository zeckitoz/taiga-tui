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
  "big_photo": "https://media.example.invalid/redacted.png",
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
  "photo": "https://media.example.invalid/redacted.png",
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
    "name": "Ask Member 2",
    "slug": "member-2-misc"
  }
]
`
	wantProject := `{
  "id": 7,
  "members": [
    {
      "full_name": "Member 2",
      "full_name_display": "Member 2",
      "id": 2,
      "username": "member-2"
    },
    {
      "full_name": "",
      "full_name_display": "Member 1",
      "id": 1,
      "username": "member-1"
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

func TestAnonymiseReplacesNamePartsAndAnyCaseInsideText(t *testing.T) {
	members := taigarec.Fixture{Name: "members.json", Body: []byte(`[
		{"id": 1, "username": "jdoe", "full_name_display": "Jane Doe"},
		{"subject": "Ask jane or DOE's JDOE account, not Janet", "jdoe": true}]`)}

	got := anonymise(t, taigarec.Anonymiser{}, members)

	want := `[
  {
    "full_name_display": "Member 1",
    "id": 1,
    "username": "member-1"
  },
  {
    "member-1": true,
    "subject": "Ask Member 1 or Member 1's member-1 account, not Janet"
  }
]
`
	if got["members.json"] != want {
		t.Errorf("got\n%s\nwant\n%s", got["members.json"], want)
	}
}

func TestAnonymiseNumbersTeamMembersByIDNotByOrder(t *testing.T) {
	// The story names Jane before any object tells her username.
	stories := taigarec.Fixture{Name: "stories.json", Body: []byte(`[
		{"assigned_to_extra_info": {"full_name_display": "Jane Doe"}}]`)}
	members := taigarec.Fixture{Name: "members.json", Body: []byte(`[
		{"id": 9, "username": "jdoe", "full_name_display": "Jane Doe"},
		{"id": 3, "username": "mmuster", "full_name_display": "Max Muster"}]`)}

	got := anonymise(t, taigarec.Anonymiser{}, stories, members)

	wantStories := `[
  {
    "assigned_to_extra_info": {
      "full_name_display": "Member 2"
    }
  }
]
`
	wantMembers := `[
  {
    "full_name_display": "Member 2",
    "id": 9,
    "username": "member-2"
  },
  {
    "full_name_display": "Member 1",
    "id": 3,
    "username": "member-1"
  }
]
`
	if got["stories.json"] != wantStories {
		t.Errorf("stories.json got\n%s\nwant\n%s", got["stories.json"], wantStories)
	}
	if got["members.json"] != wantMembers {
		t.Errorf("members.json got\n%s\nwant\n%s", got["members.json"], wantMembers)
	}
}

func TestAnonymiseReplacesUsernamesOnlyAsWholeWords(t *testing.T) {
	members := taigarec.Fixture{Name: "members.json", Body: []byte(`[
		{"id": 1, "username": "ab", "full_name_display": ""},
		{"color": "#AB12CD", "subject": "Label lab", "slug": "ab-web-shop"}]`)}

	got := anonymise(t, taigarec.Anonymiser{}, members)

	want := `[
  {
    "full_name_display": "",
    "id": 1,
    "username": "member-1"
  },
  {
    "color": "#AB12CD",
    "slug": "member-1-web-shop",
    "subject": "Label lab"
  }
]
`
	if got["members.json"] != want {
		t.Errorf("got\n%s\nwant\n%s", got["members.json"], want)
	}
}

func TestAnonymiseRedactsMediaURLs(t *testing.T) {
	project := taigarec.Fixture{Name: "project.json", Body: []byte(`{
		"logo_small_url": "https://taiga.example/media/logo.png?token=abc", "logo_big_url": null}`)}

	got := anonymise(t, taigarec.Anonymiser{}, project)

	want := `{
  "logo_big_url": null,
  "logo_small_url": "https://media.example.invalid/redacted.png"
}
`
	if got["project.json"] != want {
		t.Errorf("got\n%s\nwant\n%s", got["project.json"], want)
	}
}
