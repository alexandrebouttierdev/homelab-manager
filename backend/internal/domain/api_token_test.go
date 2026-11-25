package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestAPITokenSecret(t *testing.T) {
	token, hash, err := NewAPITokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "hlm_") || len(token) != 47 || !LooksLikeAPIToken(token) || hash != HashAPIToken(token) || len(hash) != 64 {
		t.Fatalf("token %q hash %q", token, hash)
	}
	other, _, _ := NewAPITokenSecret()
	if other == token {
		t.Fatal("two tokens are equal")
	}
	for _, bad := range []string{"", "hlm_", "hlx_" + token[4:], token + "a", token[:46], "hlm_" + strings.Repeat("!", 43)} {
		if LooksLikeAPIToken(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestAPITokenRequestValidate(t *testing.T) {
	request, err := APITokenRequest{Name: " Home Assistant ", Scopes: []APITokenScope{ScopeMetricsRead, ScopeServicesRead}}.Validate()
	if err != nil || request.Name != "Home Assistant" || request.LifetimeDays != 90 {
		t.Fatalf("request = %+v, %v", request, err)
	}
	for name, invalid := range map[string]APITokenRequest{
		"no name":       {Scopes: []APITokenScope{ScopeMetricsRead}},
		"long name":     {Name: strings.Repeat("x", 101), Scopes: []APITokenScope{ScopeMetricsRead}},
		"control":       {Name: "a\nb", Scopes: []APITokenScope{ScopeMetricsRead}},
		"no scope":      {Name: "x"},
		"unknown scope": {Name: "x", Scopes: []APITokenScope{"services:manage"}},
		"duplicate":     {Name: "x", Scopes: []APITokenScope{ScopeEventsRead, ScopeEventsRead}},
		"too long":      {Name: "x", Scopes: []APITokenScope{ScopeEventsRead}, LifetimeDays: 366},
		"negative":      {Name: "x", Scopes: []APITokenScope{ScopeEventsRead}, LifetimeDays: -1},
	} {
		if _, err := invalid.Validate(); !errors.Is(err, ErrInvalidAPIToken) {
			t.Errorf("%s accepted", name)
		}
	}
	token := APIToken{Scopes: []APITokenScope{ScopeBackupsRead}}
	if !token.Allows(ScopeBackupsRead) || token.Allows(ScopeMetricsRead) {
		t.Fatal("Allows")
	}
}
