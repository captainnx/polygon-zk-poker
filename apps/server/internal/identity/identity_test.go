package identity

import (
	"context"
	"testing"
	"time"
)

func TestAccountGuestUpgradeLoginAndLogout(t *testing.T) {
	ctx := context.Background()
	s := New(NewMemory())
	now := time.Unix(1800000000, 0)
	s.Now = func() time.Time { return now }
	guest, e := s.Guest(ctx)
	if e != nil || guest.Principal.Kind != 0 {
		t.Fatal(e)
	}
	account, e := s.Register(ctx, "Poker_Player", "test-only-password-123", &guest.Principal)
	if e != nil || account.Principal.ID != guest.Principal.ID || account.Principal.Kind != 1 {
		t.Fatal("guest identity not preserved", e)
	}
	if _, e = s.Login(ctx, "poker_player", "incorrect-password"); e == nil {
		t.Fatal("wrong password accepted")
	}
	if _, e = s.Login(ctx, "nonexistent", "incorrect-password"); e == nil {
		t.Fatal("unknown user accepted")
	}
	logged, e := s.Login(ctx, "POKER_PLAYER", "test-only-password-123")
	if e != nil || logged.Principal.ID != account.Principal.ID {
		t.Fatal("account not stable", e)
	}
	if _, e = s.Register(ctx, "poker_player", "test-only-password-456", nil); e != ErrConflict {
		t.Fatal("duplicate username accepted", e)
	}
	if _, e = s.Register(ctx, "bad user", "short", nil); e == nil {
		t.Fatal("weak credentials accepted")
	}
	if e = s.Logout(ctx, logged.Token); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Resolve(ctx, logged.Token); e == nil {
		t.Fatal("logged-out token accepted")
	}
	now = now.Add(7 * 24 * time.Hour)
	if _, e = s.Resolve(ctx, account.Token); e == nil {
		t.Fatal("expired cookie accepted")
	}
}
