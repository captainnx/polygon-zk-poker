package table

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polygon-zk-poker/server/internal/engine"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"github.com/polygon-zk-poker/server/internal/store"
	"strings"
	"sync"
	"testing"
	"time"
)

type ceremony struct {
	s      *Service
	db     *store.Memory
	spec   protocol.Spec
	keys   [9]*ecdsa.PrivateKey
	seeds  [9]common.Hash
	result Result
	now    uint64
}

func signed(t testing.TB, key *ecdsa.PrivateKey, h common.Hash) string {
	t.Helper()
	v, e := protocol.Sign(key, h)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func setup(t testing.TB, n int) *ceremony {
	t.Helper()
	ctx := context.Background()
	db := store.NewMemory()
	server, _ := crypto.GenerateKey()
	s := New(db, server, nil)
	now := uint64(1800000000)
	s.Now = func() time.Time { return time.Unix(int64(now), 0) }
	c := &ceremony{s: s, db: db, now: now}
	c.spec = protocol.Spec{Domain: protocol.Domain{ChainID: 31337, Registry: common.HexToAddress("0x1234567890123456789012345678901234567890")}, TableID: protocol.SHA([]byte("table")), HandID: protocol.SHA([]byte("hand")), AuthIssuer: s.Address(), Config: engine.Config{SmallBlind: 5, BigBlind: 10, Ante: 1}, ValidUntil: now + 600, ActionLimit: 64}
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		c.keys[i] = key
		c.spec.Config.Stacks[i] = int64(100 + i*20)
		a := protocol.Authorization{PlayerID: protocol.PlayerID(crypto.FromECDSAPub(&key.PublicKey)[1:]), AccountID: protocol.SHA([]byte{byte(i), 99}), Issuer: s.Address(), Kind: uint8(i % 2), SessionKey: crypto.PubkeyToAddress(key.PublicKey), TableID: c.spec.TableID, Nonce: uint64(i), ExpiresAt: now + 3600}
		session := protocol.Session{Domain: c.spec.Domain, Authorization: a}
		session.IssuerSignature = signed(t, s.Key, session.Digest())
		session.Signature = signed(t, key, session.Digest())
		session, e := s.Authorize(ctx, session)
		if e != nil {
			t.Fatal(e)
		}
		c.spec.Players[i] = a.PlayerID
		c.spec.Sessions[i] = session.ID
		c.seeds[i] = protocol.SHA([]byte{byte(i), 42})
	}
	r, e := s.Create(ctx, c.spec)
	if e != nil {
		t.Fatal(e)
	}
	c.result = r
	return c
}
func (c *ceremony) freeze(t testing.TB) {
	t.Helper()
	ctx := context.Background()
	for i, key := range c.keys {
		if key == nil {
			continue
		}
		commit := protocol.SeedCommit(c.spec.Context(), byte(i), c.seeds[i])
		v := protocol.SignedValue{Value: commit, Signature: signed(t, key, protocol.ValueDigest("COMMIT", c.spec.Sessions[i], c.spec.HandID, c.spec.Context(), commit))}
		r, e := c.s.Value(ctx, c.spec.HandID, i, "commit", v)
		if e != nil {
			t.Fatal(e)
		}
		c.result = r
	}
	for i, key := range c.keys {
		if key == nil {
			continue
		}
		m := c.result.Public.Manifest
		v := protocol.SignedValue{Value: m, Signature: signed(t, key, protocol.ConsentDigest(c.spec.Sessions[i], c.spec.HandID, m))}
		r, e := c.s.Value(ctx, c.spec.HandID, i, "consent", v)
		if e != nil {
			t.Fatal(e)
		}
		c.result = r
	}
}
func (c *ceremony) reveal(t testing.TB) {
	t.Helper()
	for i, key := range c.keys {
		if key == nil {
			continue
		}
		v := protocol.SignedValue{Value: c.seeds[i], Signature: signed(t, key, protocol.ValueDigest("REVEAL", c.spec.Sessions[i], c.spec.HandID, c.result.Public.Manifest, c.seeds[i]))}
		r, e := c.s.Value(context.Background(), c.spec.HandID, i, "reveal", v)
		if e != nil {
			t.Fatal(e)
		}
		c.result = r
	}
}
func (c *ceremony) move(t testing.TB) protocol.Move {
	t.Helper()
	p := c.result.Public
	h := p.Hand
	seat := h.Actor
	kind := "check"
	if h.ToCall(seat) > 0 {
		kind = "call"
	}
	if h.CanRaise(seat) {
		kind = "all-in"
	}
	m := protocol.Move{Action: engine.Action{Seat: seat, Kind: kind}, Previous: p.State, Sequence: h.Sequence + 1}
	m.PublicView, _ = protocol.PublicView(h)
	m.HoleCommit = protocol.HoleCommit(c.spec.Context(), seat, c.seeds[seat], h.Players[seat].Hole)
	digest, e := protocol.ActionDigest(c.spec.Sessions[seat], c.spec.HandID, m.Previous, m.Sequence, m.Action, m.PublicView, m.HoleCommit)
	if e != nil {
		t.Fatal(e)
	}
	m.State = digest
	m.Signature = signed(t, c.keys[seat], digest)
	return m
}
func (c *ceremony) read(t testing.TB, seat int) ReadRequest {
	t.Helper()
	x, _, e := c.s.load(context.Background(), c.spec.HandID)
	if e != nil {
		t.Fatal(e)
	}
	r := ReadRequest{SessionID: c.spec.Sessions[seat], Timestamp: c.now}
	r.Signature = signed(t, c.keys[seat], ReadDigest(x, r))
	return r
}
func TestSignedNineHandRecoveryAndAudit(t *testing.T) {
	c := setup(t, 9)
	c.freeze(t)
	c.reveal(t)
	ctx := context.Background()
	if _, e := c.s.Audit(ctx, c.spec.HandID, c.read(t, 0)); e == nil {
		t.Fatal("early entropy disclosure")
	}
	var first protocol.Move
	var firstReceipt protocol.Receipt
	for c.result.Public.Stage == "playing" {
		m := c.move(t)
		r, e := c.s.Action(ctx, c.spec.HandID, m)
		if e != nil {
			t.Fatal(e)
		}
		if first.Sequence == 0 {
			first = m
			firstReceipt = r.Receipt
		}
		c.result = r
	}
	if c.result.Public.Stage != "complete" {
		t.Fatal(c.result.Public)
	}
	retry, e := c.s.Action(ctx, c.spec.HandID, first)
	if e != nil || retry.Receipt.Digest != firstReceipt.Digest {
		t.Fatal("idempotent retry failed", e)
	}
	// New service instance recovers entirely from encrypted authoritative state.
	restarted := New(c.db, c.s.Key, nil)
	restarted.Now = c.s.Now
	c.s = restarted
	recovery, e := restarted.Recover(ctx, c.spec.HandID, c.read(t, 0), firstReceipt.Index+1, firstReceipt.Digest)
	if e != nil {
		t.Fatal(e)
	}
	if recovery.Hole == nil || len(recovery.Events) == 0 {
		t.Fatal("incomplete recovery")
	}
	if _, e = restarted.Recover(ctx, c.spec.HandID, c.read(t, 0), 1, common.Hash{}); e == nil {
		t.Fatal("fork accepted")
	}
	audit, e := c.s.Audit(ctx, c.spec.HandID, c.read(t, 0))
	if e != nil {
		t.Fatal(e)
	}
	if e = Verify(audit); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(audit)
	for name, mutate := range map[string]func(*Snapshot){"signature": func(x *Snapshot) { x.Moves[0].Signature = "0x" + strings.Repeat("00", 65) }, "output": func(x *Snapshot) { x.FinalStacks[0]++ }, "seed": func(x *Snapshot) { x.ServerSeed[0] ^= 1 }, "manifest": func(x *Snapshot) { x.Commits[0].Value[0] ^= 1 }, "chain": func(x *Snapshot) { x.Moves[0].Previous[0] ^= 1 }} {
		t.Run(name, func(t *testing.T) {
			var x Snapshot
			_ = json.Unmarshal(raw, &x)
			mutate(&x)
			if Verify(&x) == nil {
				t.Fatal("tampering accepted")
			}
		})
	}
}
func TestCommitFreezeAndForgery(t *testing.T) {
	c := setup(t, 2)
	ctx := context.Background()
	v := protocol.SignedValue{Value: c.seeds[0], Signature: signed(t, c.keys[0], protocol.ValueDigest("REVEAL", c.spec.Sessions[0], c.spec.HandID, common.Hash{}, c.seeds[0]))}
	if _, e := c.s.Value(ctx, c.spec.HandID, 0, "reveal", v); e == nil {
		t.Fatal("early reveal accepted")
	}
	c.freeze(t)
	if _, e := c.s.Value(ctx, c.spec.HandID, 0, "commit", v); e == nil {
		t.Fatal("changed commitment accepted")
	}
	c.reveal(t)
	m := c.move(t)
	original := m
	m.Signature = signed(t, c.keys[(m.Action.Seat+1)%2], m.State)
	if _, e := c.s.Action(ctx, c.spec.HandID, m); e == nil {
		t.Fatal("forged actor accepted")
	}
	m = original
	m.Previous[0] ^= 1
	if _, e := c.s.Action(ctx, c.spec.HandID, m); e == nil {
		t.Fatal("stale state accepted")
	}
}
func TestConcurrentActionOnceAndRevocation(t *testing.T) {
	c := setup(t, 2)
	c.freeze(t)
	c.reveal(t)
	m := c.move(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = c.s.Action(ctx, c.spec.HandID, m) }()
	}
	wg.Wait()
	x, _, e := c.s.load(ctx, c.spec.HandID)
	if e != nil || len(x.Moves) != 1 {
		t.Fatal(e, x)
	}
	sid := c.spec.Sessions[0]
	sig := signed(t, c.keys[0], protocol.RevokeDigest(c.spec.Domain, sid))
	if e = c.s.Revoke(ctx, sid, sig); e != nil {
		t.Fatal(e)
	}
	session, e := c.s.Store.Session(ctx, sid)
	if e != nil || !session.Revoked {
		t.Fatal(e)
	}
	session.Revoked = false
	again, e := c.s.Authorize(ctx, session)
	if e != nil || !again.Revoked {
		t.Fatal("authorization replay reactivated revoked session", e)
	}
}
func TestDeadlineAbortReceipt(t *testing.T) {
	c := setup(t, 2)
	ctx := context.Background()
	if _, e := c.s.Timeout(ctx, c.spec.HandID); e == nil {
		t.Fatal("premature timeout")
	}
	c.s.Now = func() time.Time { return time.Unix(int64(c.spec.ValidUntil), 0) }
	r, e := c.s.Timeout(ctx, c.spec.HandID)
	if e != nil || r.Public.Stage != "aborted" || r.Receipt.Verify() != nil {
		t.Fatal(e, r)
	}
}

func TestPublicPreflopBoardIsEmptyJSONArray(t *testing.T) {
	c := setup(t, 3)
	c.freeze(t)
	c.reveal(t)
	raw, e := json.Marshal(c.result.Public)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(raw), `"board":[]`) {
		t.Fatal("empty public board must be an array", string(raw))
	}
}

func TestActionBindsObservedCardsAndPublicState(t *testing.T) {
	for _, field := range []string{"board", "hole"} {
		t.Run(field, func(t *testing.T) {
			c := setup(t, 3)
			c.freeze(t)
			c.reveal(t)
			m := c.move(t)
			if field == "board" {
				m.PublicView[0] ^= 1
			} else {
				m.HoleCommit[0] ^= 1
			}
			digest, err := protocol.ActionDigest(c.spec.Sessions[m.Action.Seat], c.spec.HandID, m.Previous, m.Sequence, m.Action, m.PublicView, m.HoleCommit)
			if err != nil {
				t.Fatal(err)
			}
			m.State = digest
			m.Signature = signed(t, c.keys[m.Action.Seat], digest)
			if _, err = c.s.Action(context.Background(), c.spec.HandID, m); err == nil {
				t.Fatal("accepted valid signature over false observation")
			}
			valid := c.move(t)
			if _, err = c.s.Action(context.Background(), c.spec.HandID, valid); err != nil {
				t.Fatal("rejection mutated state", err)
			}
		})
	}
}

func TestFreshIdentitySessionCanReadButCannotActForOriginalSession(t *testing.T) {
	c := setup(t, 2)
	c.freeze(t)
	c.reveal(t)
	ctx := context.Background()
	original, err := c.db.Session(ctx, c.spec.Sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	key, _ := crypto.GenerateKey()
	fresh := original
	fresh.Authorization.SessionKey = crypto.PubkeyToAddress(key.PublicKey)
	fresh.Authorization.PlayerID = protocol.PlayerID(crypto.FromECDSAPub(&key.PublicKey)[1:])
	fresh.Authorization.Nonce++
	fresh.ID = fresh.Digest()
	fresh.IssuerSignature = signed(t, c.s.Key, fresh.ID)
	fresh.Signature = signed(t, key, fresh.Digest())
	fresh, err = c.s.Authorize(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	x, _, err := c.s.load(ctx, c.spec.HandID)
	if err != nil {
		t.Fatal(err)
	}
	req := ReadRequest{SessionID: fresh.ID, Timestamp: c.now}
	req.Signature = signed(t, key, ReadDigest(x, req))
	recovered, err := c.s.Recover(ctx, c.spec.HandID, req, 0, common.Hash{})
	if err != nil || recovered.Hole == nil {
		t.Fatal("identity recovery failed", err)
	}
	// Heads-up button seat 0 acts first. Only the roster's original session can act.
	m := c.move(t)
	m.Signature = signed(t, key, m.State)
	if _, err = c.s.Action(ctx, c.spec.HandID, m); err == nil {
		t.Fatal("replacement session took over original action authorization")
	}
}

func TestNonceConflictExpiryAndActionCapacity(t *testing.T) {
	c := setup(t, 3)
	ctx := context.Background()
	original, e := c.db.Session(ctx, c.spec.Sessions[0])
	if e != nil {
		t.Fatal(e)
	}
	alternative := original
	key, _ := crypto.GenerateKey()
	alternative.Authorization.SessionKey = crypto.PubkeyToAddress(key.PublicKey)
	alternative.Authorization.PlayerID = protocol.PlayerID(crypto.FromECDSAPub(&key.PublicKey)[1:])
	alternative.ID = alternative.Digest()
	alternative.IssuerSignature = signed(t, c.s.Key, alternative.ID)
	alternative.Signature = signed(t, key, alternative.ID)
	if _, e = c.s.Authorize(ctx, alternative); e == nil {
		t.Fatal("player/domain nonce reused for another key")
	}
	expired := original
	expired.Authorization.Nonce += 10
	expired.Authorization.ExpiresAt = c.now - 1
	expired.ID = expired.Digest()
	expired.IssuerSignature = signed(t, c.s.Key, expired.ID)
	expired.Signature = signed(t, c.keys[0], expired.ID)
	if _, e = c.s.Authorize(ctx, expired); e == nil {
		t.Fatal("expired authorization accepted")
	}
	c.spec.HandID = protocol.SHA([]byte("one-action-limit"))
	c.spec.ActionLimit = 1
	c.result, e = c.s.Create(ctx, c.spec)
	if e != nil {
		t.Fatal(e)
	}
	c.freeze(t)
	c.reveal(t)
	c.result, e = c.s.Action(ctx, c.spec.HandID, c.move(t))
	if e != nil {
		t.Fatal(e)
	}
	if c.result.Public.Stage != "aborted" || c.result.Public.AbortReason == "" {
		t.Fatal("capacity was silently truncated", c.result.Public)
	}
	if _, e = c.s.Audit(ctx, c.spec.HandID, c.read(t, 0)); e == nil {
		t.Fatal("aborted hand exported as completed audit")
	}
}

func TestIdentityCertificateCannotBeForgedBySessionKey(t *testing.T) {
	c := setup(t, 2)
	ctx := context.Background()
	original, e := c.db.Session(ctx, c.spec.Sessions[0])
	if e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"player", "kind", "issuer"} {
		t.Run(field, func(t *testing.T) {
			fake := original
			switch field {
			case "player":
				fake.Authorization.PlayerID[0] ^= 1
			case "kind":
				fake.Authorization.Kind = 1 - fake.Authorization.Kind
			case "issuer":
				fake.Authorization.Issuer = crypto.PubkeyToAddress(c.keys[0].PublicKey)
			}
			fake.ID = fake.Digest()
			fake.Signature = signed(t, c.keys[0], fake.ID)
			if _, e := c.s.Authorize(ctx, fake); e == nil {
				t.Fatal("client forged identity certificate", field)
			}
		})
	}
}

func TestEvenIssuerCannotSubstituteKeyForOriginalPlayerID(t *testing.T) {
	c := setup(t, 2)
	original, e := c.db.Session(context.Background(), c.spec.Sessions[0])
	if e != nil {
		t.Fatal(e)
	}
	key, _ := crypto.GenerateKey()
	fake := original
	fake.Authorization.SessionKey = crypto.PubkeyToAddress(key.PublicKey)
	fake.Authorization.Nonce++
	fake.ID = fake.Digest()
	fake.IssuerSignature = signed(t, c.s.Key, fake.ID)
	fake.Signature = signed(t, key, fake.ID)
	if _, e = c.s.Authorize(context.Background(), fake); e == nil {
		t.Fatal("issuer impersonated an existing player's public key ID")
	}
}
