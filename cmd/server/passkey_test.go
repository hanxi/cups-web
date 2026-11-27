package main

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestParsePasskeyConfig(t *testing.T) {
	if cfg, err := parsePasskeyConfig(envMap(nil)); err != nil || cfg.Enabled {
		t.Fatalf("disabled by default: %+v %v", cfg, err)
	}
	bad := []map[string]string{
		{"PASSKEY_ENABLED": "true"},
		{"PASSKEY_ENABLED": "true", "PASSKEY_RP_ID": "192.168.1.2", "PASSKEY_RP_ORIGINS": "https://a.com"},
		{"PASSKEY_ENABLED": "true", "PASSKEY_RP_ID": "a.com:443", "PASSKEY_RP_ORIGINS": "https://a.com"},
		{"PASSKEY_ENABLED": "true", "PASSKEY_RP_ID": "a.com"},
	}
	for _, m := range bad {
		if _, err := parsePasskeyConfig(envMap(m)); err == nil {
			t.Fatalf("expected error for %v", m)
		}
	}
	cfg, err := parsePasskeyConfig(envMap(map[string]string{
		"PASSKEY_ENABLED": "1", "PASSKEY_RP_ID": "print.example.com", "TRUSTED_ORIGINS": "https://print.example.com/, https://b.com",
	}))
	if err != nil || len(cfg.Origins) != 2 || cfg.Origins[0] != "https://print.example.com" || cfg.RPName != "CUPS Web" {
		t.Fatalf("fallback to TRUSTED_ORIGINS failed: %+v %v", cfg, err)
	}
	cfg, _ = parsePasskeyConfig(envMap(map[string]string{
		"PASSKEY_ENABLED": "1", "PASSKEY_RP_ID": "a.com", "PASSKEY_RP_ORIGINS": "https://a.com", "TRUSTED_ORIGINS": "https://x.com",
	}))
	if len(cfg.Origins) != 1 || cfg.Origins[0] != "https://a.com" {
		t.Fatalf("PASSKEY_RP_ORIGINS should win: %+v", cfg)
	}
}

func TestPasskeyChallengeStore(t *testing.T) {
	s := newPasskeyChallengeStore()
	now := time.Now()
	s.now = func() time.Time { return now }
	k, ok := s.put(passkeyChallenge{kind: "login", session: webauthn.SessionData{Challenge: "c"}})
	if !ok {
		t.Fatal("put failed")
	}
	if _, ok := s.take(k, "register"); ok {
		t.Fatal("kind mismatch must fail")
	}
	if _, ok := s.take(k, "login"); ok {
		t.Fatal("take must be one-shot (mismatch already consumed)")
	}
	k, _ = s.put(passkeyChallenge{kind: "login"})
	if _, ok := s.take(k, "login"); !ok {
		t.Fatal("first take should succeed")
	}
	if _, ok := s.take(k, "login"); ok {
		t.Fatal("second take must fail")
	}
	k, _ = s.put(passkeyChallenge{kind: "login"})
	now = now.Add(passkeyChallengeTTL + time.Second)
	if _, ok := s.take(k, "login"); ok {
		t.Fatal("expired must fail")
	}
}
