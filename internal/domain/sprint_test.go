package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/zeckitoz/taiga-tui/internal/domain"
)

func date(y int, m time.Month, d int) domain.Date { return domain.Date{Year: y, Month: m, Day: d} }

// Sprints of a project, newest first as Taiga lists them.
var (
	s5 = domain.Sprint{ID: 5, Name: "Sprint 5", Start: date(2026, 11, 17), Finish: date(2026, 11, 30)}
	s4 = domain.Sprint{ID: 4, Name: "Sprint 4", Start: date(2026, 11, 3), Finish: date(2026, 11, 16)}
	// s4b overlaps s4 and was closed early.
	s4b = domain.Sprint{ID: 40, Name: "Hotfix", Start: date(2026, 11, 10), Finish: date(2026, 11, 12), Closed: true}
	s3  = domain.Sprint{ID: 3, Name: "Sprint 3", Start: date(2026, 10, 20), Finish: date(2026, 11, 2), Closed: true}
	s2  = domain.Sprint{ID: 2, Name: "Sprint 2", Start: date(2026, 10, 6), Finish: date(2026, 10, 19)}
)

func TestResolveCurrentSprint(t *testing.T) {
	tests := []struct {
		name       string
		sprints    []domain.Sprint
		today      domain.Date
		remembered int64
		want       domain.Resolution
	}{
		{
			name:    "today inside one Sprint",
			sprints: []domain.Sprint{s5, s4, s3},
			today:   date(2026, 11, 5),
			want:    domain.Resolution{Kind: domain.One, Sprint: s4},
		},
		{
			name:    "first and last day count",
			sprints: []domain.Sprint{s5, s4, s3},
			today:   date(2026, 11, 16),
			want:    domain.Resolution{Kind: domain.One, Sprint: s4},
		},
		{
			name:    "closed is ignored",
			sprints: []domain.Sprint{s5, s4, s3},
			today:   date(2026, 10, 25),
			want:    domain.Resolution{Kind: domain.One, Sprint: s3},
		},
		{
			name:       "a remembered Sprint does not override a single match",
			sprints:    []domain.Sprint{s5, s4, s3},
			today:      date(2026, 11, 5),
			remembered: 5,
			want:       domain.Resolution{Kind: domain.One, Sprint: s4},
		},
		{
			name:    "several matching Sprints, newest first",
			sprints: []domain.Sprint{s4, s4b, s3},
			today:   date(2026, 11, 11),
			want:    domain.Resolution{Kind: domain.Choose, Reason: domain.SeveralCurrent, Choices: []domain.Sprint{s4b, s4}},
		},
		{
			name:       "several matching Sprints with the remembered one among them",
			sprints:    []domain.Sprint{s4, s4b, s3},
			today:      date(2026, 11, 11),
			remembered: 40,
			want:       domain.Resolution{Kind: domain.One, Sprint: s4b},
		},
		{
			name:       "remembered Sprint outside the list is not used",
			sprints:    []domain.Sprint{s4, s4b, s3},
			today:      date(2026, 11, 11),
			remembered: 3,
			want:       domain.Resolution{Kind: domain.Choose, Reason: domain.SeveralCurrent, Choices: []domain.Sprint{s4b, s4}},
		},
		{
			name:    "none matching offers the open Sprints",
			sprints: []domain.Sprint{s5, s3, s2},
			today:   date(2026, 11, 5),
			want:    domain.Resolution{Kind: domain.Choose, Reason: domain.NoneCurrent, Choices: []domain.Sprint{s5, s2}},
		},
		{
			name:       "none matching with the remembered open Sprint",
			sprints:    []domain.Sprint{s5, s3, s2},
			today:      date(2026, 11, 5),
			remembered: 2,
			want:       domain.Resolution{Kind: domain.One, Sprint: s2},
		},
		{
			name:    "none matching and none open offers all Sprints, newest first",
			sprints: []domain.Sprint{s3, s4b},
			today:   date(2027, 1, 1),
			want:    domain.Resolution{Kind: domain.Choose, Reason: domain.NoneOpen, Choices: []domain.Sprint{s4b, s3}},
		},
		{
			name:  "no Sprints",
			today: date(2026, 11, 5),
			want:  domain.Resolution{Kind: domain.None},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.ResolveCurrentSprint(tt.sprints, tt.today, tt.remembered)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestDateOfUsesTheTimesLocation(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	// 23:30 UTC on 2 November is already 3 November in Berlin.
	at := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC).In(berlin)

	if got := domain.DateOf(at); got != date(2026, 11, 3) {
		t.Errorf("got %+v", got)
	}
}

func TestSprintContains(t *testing.T) {
	if !s4.Contains(date(2026, 11, 3)) || !s4.Contains(date(2026, 11, 16)) {
		t.Error("first or last day not contained")
	}
	if s4.Contains(date(2026, 11, 2)) || s4.Contains(date(2026, 11, 17)) {
		t.Error("day outside contained")
	}
}

func TestNewestFirstSortsByStartWithoutChangingInput(t *testing.T) {
	in := []domain.Sprint{s2, s5, s3}

	got := domain.NewestFirst(in)

	if want := []domain.Sprint{s5, s3, s2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	if in[0] != s2 {
		t.Error("input changed")
	}
}
