package taiga_test

import (
	"cmp"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/config"
	"github.com/zeckitoz/taiga-tui/internal/domain"
	"github.com/zeckitoz/taiga-tui/internal/taiga"
	"github.com/zeckitoz/taiga-tui/internal/taiga/taigarec"
)

var record = flag.Bool("record", false, "record the fixtures in testdata from the live Taiga test project")

// TestRecordFixtures replaces the fixtures in testdata with anonymised
// responses of the live instance. It runs only with -record:
//
//	TAIGA_TUI_LIVE_USER=… TAIGA_TUI_LIVE_PASSWORD=… TAIGA_TUI_LIVE_PROJECT=<slug> \
//	  go test ./internal/taiga -run TestRecordFixtures -record
//
// TAIGA_TUI_LIVE_URL overrides the instance, which defaults to Ostfalia's.
// Without -record all tests use the checked-in fixtures.
func TestRecordFixtures(t *testing.T) {
	if !*record {
		t.Skip("records from the live instance only with -record")
	}
	user, password, slug := liveEnv(t)
	instance := config.DefaultInstanceURL
	if u := os.Getenv("TAIGA_TUI_LIVE_URL"); u != "" {
		instance = u
	}
	rec := taigarec.NewRecorder(http.DefaultTransport)
	auth := taiga.NewAuthAPI(instance, taiga.WithTransport(rec))
	ctx := t.Context()

	var login taiga.LoginResult
	mustRecord(t, rec, "login.json", func() (err error) {
		login, err = auth.Login(ctx, user, password)
		return err
	})
	var tokens taiga.Tokens
	mustRecord(t, rec, "refresh.json", func() (err error) {
		tokens, err = auth.Refresh(ctx, login.Tokens.Refresh)
		return err
	})

	client := taiga.NewClient(instance, staticToken(tokens.Access), taiga.WithTransport(rec))
	var me domain.TeamMember
	mustRecord(t, rec, "me.json", func() (err error) {
		me, err = client.Me(ctx)
		return err
	})
	var projects []domain.ProjectSummary
	mustRecord(t, rec, "projects.json", func() (err error) {
		projects, err = client.Projects(ctx, me.ID)
		return err
	})
	var project domain.Project
	mustRecord(t, rec, "project_by_slug.json", func() (err error) {
		project, err = client.Project(ctx, slug)
		return err
	})
	mustRecord(t, rec, "userstories_kanban.json", func() error {
		_, err := client.Kanban(ctx, project.ID)
		return err
	})

	anon := taigarec.Anonymiser{Replace: projectNames(projects, project)}
	if err := rec.Save("testdata", anon, user, password, me.Username, me.FullName); err != nil {
		t.Fatal(err)
	}
}

func liveEnv(t *testing.T) (user, password, slug string) {
	t.Helper()
	vars := []string{"TAIGA_TUI_LIVE_USER", "TAIGA_TUI_LIVE_PASSWORD", "TAIGA_TUI_LIVE_PROJECT"}
	values := make([]string, len(vars))
	for i, name := range vars {
		values[i] = os.Getenv(name)
		if values[i] == "" {
			t.Fatalf("-record needs %s", name)
		}
	}
	return values[0], values[1], values[2]
}

func mustRecord(t *testing.T, rec *taigarec.Recorder, name string, call func() error) {
	t.Helper()
	if err := rec.Record(name, call); err != nil {
		t.Fatal(err)
	}
}

// projectNames hides the names and slugs of the recorded project and of every
// other project the Team Member belongs to, numbered by project ID so that a
// new recording keeps the numbers.
func projectNames(projects []domain.ProjectSummary, recorded domain.Project) map[string]string {
	projects = slices.SortedFunc(slices.Values(projects), func(a, b domain.ProjectSummary) int {
		return cmp.Compare(a.ID, b.ID)
	})
	replace := map[string]string{}
	for i, p := range projects {
		replace[p.Name] = fmt.Sprintf("Project %d", i+1)
		replace[p.Slug] = fmt.Sprintf("project-%d", i+1)
	}
	replace[recorded.Name] = "Test Project"
	replace[recorded.Slug] = "test-project"
	return replace
}
