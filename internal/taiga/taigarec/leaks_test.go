package taigarec_test

import (
	"reflect"
	"testing"

	"github.com/zeckitoz/taiga-tui/internal/taiga/taigarec"
)

func TestLeaksFindsEmailAddressesAndJWTs(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjo0Mn0.c2lnbmF0dXJl"
	body := []byte(`{"email": "jane.doe+taiga@uni.example", "auth_token": "` + jwt + `", "note": "ping @jane"}`)

	got := taigarec.Leaks(body)

	want := []string{"jane.doe+taiga@uni.example", jwt}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLeaksAcceptsCleanJSON(t *testing.T) {
	body := []byte(`{"email": "redacted", "auth_token": "redacted-access-token-1", "subject": "Mention @team in the README"}`)

	if got := taigarec.Leaks(body); len(got) != 0 {
		t.Errorf("got %q, want none", got)
	}
}
