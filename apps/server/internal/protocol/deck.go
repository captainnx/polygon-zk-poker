package protocol

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polygon-zk-poker/server/internal/engine"
	"sort"
)

// Hash-sort is the v2 deterministic shuffle. Each card is assigned a 256-bit
// random-oracle priority; ties break by card ID. No modulo reduction or retry
// selection occurs. Collision/tie bias is bounded by 1326/2^256.
func Deck(context common.Hash, server common.Hash, reveals [9]SignedValue, occupied [9]bool) ([52]engine.Card, common.Hash) {
	b := append([]byte("ZKPOKER_MIX_V3"), context[:]...)
	b = append(b, server[:]...)
	for i, p := range reveals {
		if occupied[i] {
			b = append(b, byte(i))
			b = append(b, p.Value[:]...)
		}
	}
	key := SHA(b)
	var priorities [52]common.Hash
	var deck [52]engine.Card
	for i := range deck {
		deck[i] = engine.Card(i)
		priorities[i] = SHA([]byte("ZKPOKER_CARD_V3"), key[:], []byte{byte(i)})
	}
	sort.Slice(deck[:], func(i, j int) bool {
		a, c := deck[i], deck[j]
		cmp := bytes.Compare(priorities[a][:], priorities[c][:])
		return cmp < 0 || (cmp == 0 && a < c)
	})
	raw := make([]byte, 52)
	for i, c := range deck {
		raw[i] = byte(c)
	}
	return deck, SHA([]byte("ZKPOKER_DECK_V3"), context[:], raw)
}
func ValidateSpec(spec Spec, sessions [9]Session, now uint64) error {
	if e := spec.Domain.Validate(); e != nil {
		return e
	}
	if spec.HandID == (common.Hash{}) || spec.TableID == (common.Hash{}) || spec.ValidUntil <= now || spec.ValidUntil > now+3600 || spec.ActionLimit == 0 || spec.ActionLimit > MaxActions {
		return fmt.Errorf("invalid hand context, deadline or action limit")
	}
	var identity [52]engine.Card
	for i := range identity {
		identity[i] = engine.Card(i)
	}
	if _, e := engine.New(spec.Config, identity); e != nil {
		return e
	}
	seen := map[common.Hash]bool{}
	for i, stack := range spec.Config.Stacks {
		if stack == 0 {
			if spec.Players[i] != (common.Hash{}) || spec.Sessions[i] != (common.Hash{}) {
				return fmt.Errorf("nonempty vacant seat")
			}
			continue
		}
		s := sessions[i]
		a := s.Authorization
		if s.ID != spec.Sessions[i] || s.Digest() != s.ID || s.Revoked || a.ExpiresAt < spec.ValidUntil || a.PlayerID != spec.Players[i] || a.TableID != spec.TableID || s.Domain != spec.Domain || seen[a.AccountID] {
			return fmt.Errorf("invalid session at seat %d", i)
		}
		if a.Kind > 1 || a.Issuer != spec.AuthIssuer || spec.AuthIssuer == (common.Address{}) {
			return fmt.Errorf("identity issuer mismatch")
		}
		if e := Verify(s.ID, s.IssuerSignature, spec.AuthIssuer); e != nil {
			return e
		}
		if a.AccountID == (common.Hash{}) {
			return fmt.Errorf("empty account identity")
		}
		if e := s.ValidateOwnership(); e != nil {
			return e
		}
		seen[a.AccountID] = true
	}
	return nil
}
