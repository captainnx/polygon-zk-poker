package table

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polygon-zk-poker/server/internal/engine"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"github.com/polygon-zk-poker/server/internal/store"
	"github.com/redis/go-redis/v9"
)

type Snapshot struct {
	Version      string                  `json:"version"`
	Spec         protocol.Spec           `json:"spec"`
	Sessions     [9]protocol.Session     `json:"authorizations"`
	Server       common.Address          `json:"server"`
	ServerSeed   common.Hash             `json:"serverSeed"`
	ServerCommit common.Hash             `json:"serverCommit"`
	Commits      [9]protocol.SignedValue `json:"commits"`
	Consents     [9]string               `json:"consents"`
	Reveals      [9]protocol.SignedValue `json:"reveals"`
	Manifest     common.Hash             `json:"manifest"`
	DeckCommit   common.Hash             `json:"deckCommit"`
	State        common.Hash             `json:"state"`
	Moves        []protocol.Move         `json:"moves"`
	MoveReceipts []protocol.Receipt      `json:"moveReceipts"`
	FinalStacks  [9]int64                `json:"finalStacks"`
	Stage        string                  `json:"stage"`
	AbortReason  string                  `json:"abortReason,omitempty"`
	LastReceipt  common.Hash             `json:"lastReceipt"`
}
type Public struct {
	Spec         protocol.Spec  `json:"spec"`
	Server       common.Address `json:"server"`
	ServerCommit common.Hash    `json:"serverCommit"`
	Commits      [9]common.Hash `json:"commits"`
	Consented    [9]bool        `json:"consented"`
	Revealed     [9]bool        `json:"revealed"`
	Manifest     common.Hash    `json:"manifest"`
	DeckCommit   common.Hash    `json:"deckCommit"`
	State        common.Hash    `json:"state"`
	Stage        string         `json:"stage"`
	AbortReason  string         `json:"abortReason,omitempty"`
	Version      uint64         `json:"version,string"`
	LastReceipt  common.Hash    `json:"lastReceipt"`
	Hand         *engine.Hand   `json:"hand,omitempty"`
	ToCall       int64          `json:"toCall"`
	CanRaise     bool           `json:"canRaise"`
	MinTarget    int64          `json:"minTarget"`
}
type Result struct {
	Public  Public           `json:"public"`
	Receipt protocol.Receipt `json:"receipt"`
}
type ReadRequest struct {
	SessionID common.Hash `json:"sessionId"`
	Timestamp uint64      `json:"timestamp,string"`
	Signature string      `json:"signature"`
}
type Recovery struct {
	Public Public             `json:"public"`
	Events []protocol.Receipt `json:"events"`
	Hole   *[2]engine.Card    `json:"hole,omitempty"`
}
type Service struct {
	Store store.Store
	Key   *ecdsa.PrivateKey
	Cache *redis.Client
	Now   func() time.Time
	aead  cipher.AEAD
}

func LoadKey(path string) (*ecdsa.PrivateKey, error) {
	b, e := os.ReadFile(path)
	if e == nil {
		return crypto.ToECDSA(b)
	}
	if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	key, e := crypto.GenerateKey()
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	_, e = f.Write(crypto.FromECDSA(key))
	ce := f.Close()
	if e != nil {
		return nil, e
	}
	return key, ce
}
func New(db store.Store, key *ecdsa.PrivateKey, cache *redis.Client) *Service {
	storageKey := protocol.SHA([]byte("ZKPOKER_STORAGE_V3"), crypto.FromECDSA(key))
	block, e := aes.NewCipher(storageKey[:])
	if e != nil {
		panic(e)
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		panic(e)
	}
	return &Service{db, key, cache, time.Now, aead}
}
func (s *Service) now() uint64             { return uint64(s.Now().Unix()) }
func (s *Service) Address() common.Address { return crypto.PubkeyToAddress(s.Key.PublicKey) }
func (s *Service) encode(x *Snapshot) ([]byte, error) {
	raw, e := json.Marshal(x)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return s.aead.Seal(nonce, nonce, raw, x.Spec.HandID[:]), nil
}
func (s *Service) load(ctx context.Context, id common.Hash) (*Snapshot, uint64, error) {
	h, e := s.Store.Load(ctx, id)
	if e != nil {
		return nil, 0, e
	}
	if len(h.Snapshot) < s.aead.NonceSize() {
		return nil, 0, fmt.Errorf("corrupt snapshot")
	}
	n := s.aead.NonceSize()
	raw, e := s.aead.Open(nil, h.Snapshot[:n], h.Snapshot[n:], id[:])
	if e != nil {
		return nil, 0, fmt.Errorf("snapshot authentication failed")
	}
	var x Snapshot
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, 0, e
	}
	if x.Spec.HandID != id || x.Server != s.Address() {
		return nil, 0, fmt.Errorf("snapshot identity mismatch")
	}
	return &x, h.Version, nil
}
func (s *Service) Authorize(ctx context.Context, session protocol.Session) (protocol.Session, error) {
	if session.Authorization.Issuer != s.Address() {
		return session, fmt.Errorf("untrusted identity issuer")
	}
	if e := session.Validate(s.now()); e != nil {
		return session, e
	}
	if e := s.Store.PutSession(ctx, session); e != nil {
		return session, e
	}
	return s.Store.Session(ctx, session.ID)
}
func (s *Service) Revoke(ctx context.Context, id common.Hash, signature string) error {
	session, e := s.Store.Session(ctx, id)
	if e != nil {
		return e
	}
	if e = protocol.Verify(protocol.RevokeDigest(session.Domain, id), signature, session.Authorization.SessionKey); e != nil {
		return e
	}
	return s.Store.Revoke(ctx, id)
}
func ids(x *Snapshot) []common.Hash {
	out := []common.Hash{}
	for i, stack := range x.Spec.Config.Stacks {
		if stack > 0 {
			out = append(out, x.Spec.Sessions[i])
		}
	}
	return out
}
func occupied(x *Snapshot) [9]bool {
	var o [9]bool
	for i, stack := range x.Spec.Config.Stacks {
		o[i] = stack > 0
	}
	return o
}
func all(x *Snapshot, pred func(int) bool) bool {
	for i, stack := range x.Spec.Config.Stacks {
		if stack > 0 && !pred(i) {
			return false
		}
	}
	return true
}
func (s *Service) receipt(x *Snapshot, index uint64, kind string, payload any) (protocol.Receipt, error) {
	b, e := json.Marshal(payload)
	if e != nil {
		return protocol.Receipt{}, e
	}
	r := protocol.Receipt{HandID: x.Spec.HandID, Index: index, Kind: kind, Payload: b, Previous: x.LastReceipt, At: s.now(), Server: s.Address()}
	r.Digest = r.Hash()
	r.Signature, e = protocol.Sign(s.Key, r.Digest)
	if e == nil {
		x.LastReceipt = r.Digest
	}
	return r, e
}
func public(x *Snapshot, version uint64) (Public, error) {
	p := Public{Spec: x.Spec, Server: x.Server, ServerCommit: x.ServerCommit, Manifest: x.Manifest, DeckCommit: x.DeckCommit, State: x.State, Stage: x.Stage, AbortReason: x.AbortReason, Version: version, LastReceipt: x.LastReceipt}
	for i := 0; i < 9; i++ {
		p.Commits[i] = x.Commits[i].Value
		p.Consented[i] = x.Consents[i] != ""
		p.Revealed[i] = x.Reveals[i].Signature != ""
	}
	if x.Stage == "playing" || x.Stage == "complete" {
		h, e := Rebuild(x)
		if e != nil {
			return p, e
		}
		if h.Board == nil {
			h.Board = []engine.Card{}
		}
		p.Hand = h
		p.ToCall = h.ToCall(h.Actor)
		p.CanRaise = h.CanRaise(h.Actor)
		p.MinTarget = h.CurrentBet + h.MinRaise
	}
	return p, nil
}
func (s *Service) Public(ctx context.Context, id common.Hash) (Public, error) {
	x, version, err := s.load(ctx, id)
	if err != nil {
		return Public{}, err
	}
	return public(x, version)
}
func (s *Service) cache(ctx context.Context, p Public) {
	if s.Cache == nil {
		return
	}
	b, e := json.Marshal(p)
	if e == nil {
		_ = s.Cache.Set(ctx, "poker:v2:"+p.Spec.HandID.Hex(), b, 15*time.Minute).Err()
	}
}
func (s *Service) Create(ctx context.Context, spec protocol.Spec) (Result, error) {
	if old, v, e := s.load(ctx, spec.HandID); e == nil {
		if old.Spec != spec {
			return Result{}, store.ErrConflict
		}
		p, e := public(old, v)
		if e != nil {
			return Result{}, e
		}
		events, e := s.Store.Events(ctx, spec.HandID, 0)
		if e != nil {
			return Result{}, e
		}
		return Result{p, events[0]}, nil
	} else if !errors.Is(e, store.ErrNotFound) {
		return Result{}, e
	}
	if spec.AuthIssuer != s.Address() {
		return Result{}, fmt.Errorf("untrusted hand identity issuer")
	}
	x := &Snapshot{Version: "signed-hand-v3", Spec: spec, Server: s.Address(), Stage: "committing", Moves: []protocol.Move{}, MoveReceipts: []protocol.Receipt{}}
	for i, stack := range spec.Config.Stacks {
		if stack > 0 {
			session, e := s.Store.Session(ctx, spec.Sessions[i])
			if e != nil {
				return Result{}, e
			}
			x.Sessions[i] = session
		}
	}
	if e := protocol.ValidateSpec(spec, x.Sessions, s.now()); e != nil {
		return Result{}, e
	}
	if _, e := rand.Read(x.ServerSeed[:]); e != nil {
		return Result{}, e
	}
	x.ServerCommit = protocol.SeedCommit(spec.Context(), 255, x.ServerSeed)
	r, e := s.receipt(x, 0, "hand-created", map[string]any{"context": spec.Context(), "spec": spec, "serverCommit": x.ServerCommit})
	if e != nil {
		return Result{}, e
	}
	encoded, e := s.encode(x)
	if e != nil {
		return Result{}, e
	}
	if e = s.Store.Create(ctx, spec.HandID, encoded, r, ids(x), s.now()); e != nil {
		return Result{}, e
	}
	p, e := public(x, 0)
	s.cache(ctx, p)
	return Result{p, r}, e
}

type mutation func(*Snapshot, uint64) (string, any, []common.Hash, *protocol.Receipt, error)

func (s *Service) mutate(ctx context.Context, id common.Hash, fn mutation) (Result, error) {
	for attempt := 0; attempt < 5; attempt++ {
		x, version, e := s.load(ctx, id)
		if e != nil {
			return Result{}, e
		}
		kind, payload, required, duplicate, e := fn(x, version)
		if e != nil {
			return Result{}, e
		}
		if duplicate != nil {
			p, e := public(x, version)
			return Result{p, *duplicate}, e
		}
		r, e := s.receipt(x, version+1, kind, payload)
		if e != nil {
			return Result{}, e
		}
		if kind == "action" {
			x.MoveReceipts = append(x.MoveReceipts, r)
		}
		encoded, e := s.encode(x)
		if e != nil {
			return Result{}, e
		}
		e = s.Store.Commit(ctx, id, version, encoded, r, required, s.now())
		if errors.Is(e, store.ErrConflict) {
			continue
		}
		if e != nil {
			return Result{}, e
		}
		p, e := public(x, version+1)
		s.cache(ctx, p)
		return Result{p, r}, e
	}
	return Result{}, store.ErrConflict
}
func (s *Service) signer(x *Snapshot, seat int) (common.Address, error) {
	if seat < 0 || seat >= 9 || x.Spec.Config.Stacks[seat] == 0 {
		return common.Address{}, fmt.Errorf("invalid seat")
	}
	if s.now() >= x.Spec.ValidUntil {
		return common.Address{}, fmt.Errorf("hand expired; request timeout receipt")
	}
	return x.Sessions[seat].Authorization.SessionKey, nil
}
func (s *Service) Value(ctx context.Context, id common.Hash, seat int, kind string, value protocol.SignedValue) (Result, error) {
	return s.mutate(ctx, id, func(x *Snapshot, v uint64) (string, any, []common.Hash, *protocol.Receipt, error) {
		signer, e := s.signer(x, seat)
		if e != nil {
			return "", nil, nil, nil, e
		}
		sid := x.Spec.Sessions[seat]
		contextHash := x.Spec.Context()
		expectedStage := ""
		var previous protocol.SignedValue
		switch kind {
		case "commit":
			expectedStage = "committing"
			previous = x.Commits[seat]
			e = protocol.Verify(protocol.ValueDigest("COMMIT", sid, id, contextHash, value.Value), value.Signature, signer)
		case "consent":
			expectedStage = "consenting"
			previous = protocol.SignedValue{Value: x.Manifest, Signature: x.Consents[seat]}
			if value.Value != x.Manifest {
				e = fmt.Errorf("manifest mismatch")
			} else {
				e = protocol.Verify(protocol.ConsentDigest(sid, id, x.Manifest), value.Signature, signer)
			}
		case "reveal":
			expectedStage = "revealing"
			previous = x.Reveals[seat]
			e = protocol.Verify(protocol.ValueDigest("REVEAL", sid, id, x.Manifest, value.Value), value.Signature, signer)
			if e == nil && protocol.SeedCommit(contextHash, byte(seat), value.Value) != x.Commits[seat].Value {
				e = fmt.Errorf("seed does not open commitment")
			}
		default:
			e = fmt.Errorf("invalid ceremony command")
		}
		if e != nil {
			return "", nil, nil, nil, e
		}
		if previous.Signature != "" {
			if previous.Value != value.Value {
				return "", nil, nil, nil, store.ErrConflict
			}
			events, e := s.Store.Events(ctx, id, 0)
			if e != nil {
				return "", nil, nil, nil, e
			}
			for _, r := range events {
				var payload struct {
					Seat int `json:"seat"`
				}
				if r.Kind == kind && json.Unmarshal(r.Payload, &payload) == nil && payload.Seat == seat {
					return "", nil, nil, &r, nil
				}
			}
			return "", nil, nil, nil, fmt.Errorf("missing ceremony receipt")
		}
		if x.Stage != expectedStage {
			return "", nil, nil, nil, fmt.Errorf("expected stage %s; got %s", expectedStage, x.Stage)
		}
		switch kind {
		case "commit":
			x.Commits[seat] = value
			if all(x, func(i int) bool { return x.Commits[i].Signature != "" }) {
				x.Manifest = protocol.Manifest(contextHash, x.ServerCommit, x.Commits)
				x.Stage = "consenting"
			}
		case "consent":
			x.Consents[seat] = value.Signature
			if all(x, func(i int) bool { return x.Consents[i] != "" }) {
				x.Stage = "revealing"
			}
		case "reveal":
			x.Reveals[seat] = value
			if all(x, func(i int) bool { return x.Reveals[i].Signature != "" }) {
				_, x.DeckCommit = protocol.Deck(contextHash, x.ServerSeed, x.Reveals, occupied(x))
				x.State = protocol.InitialState(contextHash, x.Manifest, x.DeckCommit)
				x.Stage = "playing"
				h, e := Rebuild(x)
				if e != nil {
					return "", nil, nil, nil, e
				}
				finish(x, h)
			}
		}
		// Reveal receipts never contain seeds; disclosure occurs only after Complete.
		committed := value.Value
		if kind == "reveal" {
			committed = x.Commits[seat].Value
		}
		return kind, map[string]any{"seat": seat, "value": committed, "manifest": x.Manifest, "stage": x.Stage, "state": x.State, "deckCommit": x.DeckCommit, "signature": value.Signature, "sessionId": sid}, []common.Hash{sid}, nil, nil
	})
}
func finish(x *Snapshot, h *engine.Hand) {
	if h.Street == engine.Complete {
		x.Stage = "complete"
		for i, p := range h.Players {
			x.FinalStacks[i] = p.Stack
		}
	}
}
func Rebuild(x *Snapshot) (*engine.Hand, error) {
	d, commit := protocol.Deck(x.Spec.Context(), x.ServerSeed, x.Reveals, occupied(x))
	if commit != x.DeckCommit {
		return nil, fmt.Errorf("deck commitment mismatch")
	}
	h, e := engine.New(x.Spec.Config, d)
	if e != nil {
		return nil, e
	}
	for _, m := range x.Moves {
		if e = h.Apply(m.Action); e != nil {
			return nil, e
		}
	}
	return h, nil
}
func (s *Service) Action(ctx context.Context, id common.Hash, m protocol.Move) (Result, error) {
	return s.mutate(ctx, id, func(x *Snapshot, v uint64) (string, any, []common.Hash, *protocol.Receipt, error) {
		if m.Sequence > 0 && m.Sequence <= uint64(len(x.Moves)) {
			old := x.Moves[m.Sequence-1]
			if old.Action == m.Action && old.Previous == m.Previous && old.Signature == m.Signature && old.PublicView == m.PublicView && old.HoleCommit == m.HoleCommit {
				return "", nil, nil, &x.MoveReceipts[m.Sequence-1], nil
			}
			return "", nil, nil, nil, store.ErrConflict
		}
		if x.Stage != "playing" || len(x.Moves) >= int(x.Spec.ActionLimit) || m.Sequence != uint64(len(x.Moves)+1) || m.Previous != x.State {
			return "", nil, nil, nil, fmt.Errorf("invalid stage, sequence, previous state or action capacity")
		}
		signer, e := s.signer(x, m.Action.Seat)
		if e != nil {
			return "", nil, nil, nil, e
		}
		sid := x.Spec.Sessions[m.Action.Seat]
		h, e := Rebuild(x)
		if e != nil {
			return "", nil, nil, nil, e
		}
		view, e := protocol.PublicView(h)
		if e != nil || view != m.PublicView || protocol.HoleCommit(x.Spec.Context(), m.Action.Seat, x.Reveals[m.Action.Seat].Value, h.Players[m.Action.Seat].Hole) != m.HoleCommit {
			return "", nil, nil, nil, fmt.Errorf("player observation mismatch")
		}
		digest, e := protocol.ActionDigest(sid, id, m.Previous, m.Sequence, m.Action, m.PublicView, m.HoleCommit)
		if e != nil {
			return "", nil, nil, nil, e
		}
		if m.State != (common.Hash{}) && m.State != digest {
			return "", nil, nil, nil, fmt.Errorf("claimed state mismatch")
		}
		if e = protocol.Verify(digest, m.Signature, signer); e != nil {
			return "", nil, nil, nil, e
		}
		if e = h.Apply(m.Action); e != nil {
			return "", nil, nil, nil, e
		}
		m.State = digest
		x.State = digest
		x.Moves = append(x.Moves, m)
		finish(x, h)
		if x.Stage == "playing" && len(x.Moves) == int(x.Spec.ActionLimit) {
			x.Stage = "aborted"
			x.AbortReason = "signed action capacity exhausted; no result may be registered"
		}
		return "action", m, []common.Hash{sid}, nil, nil
	})
}
func ReadDigest(x *Snapshot, r ReadRequest) common.Hash {
	var value common.Hash
	for i := 0; i < 8; i++ {
		value[31-i] = byte(r.Timestamp >> (8 * i))
	}
	return protocol.ValueDigest("READ", r.SessionID, x.Spec.HandID, x.Spec.Context(), value)
}
func (s *Service) readAuth(ctx context.Context, x *Snapshot, r ReadRequest) (int, error) {
	now := s.now()
	if r.Timestamp > now+30 || now > r.Timestamp+30 {
		return -1, fmt.Errorf("read authorization expired")
	}
	session, e := s.Store.Session(ctx, r.SessionID)
	if e != nil {
		return -1, e
	}
	if session.Revoked || session.Authorization.ExpiresAt <= now {
		return -1, fmt.Errorf("inactive read session")
	}
	if session.Domain != x.Spec.Domain || session.Authorization.TableID != x.Spec.TableID {
		return -1, fmt.Errorf("read domain mismatch")
	}
	// A newly authenticated session may read the same owner's history, but
	// action signing remains permanently bound to the original roster session.
	for i := range x.Spec.Players {
		if x.Sessions[i].Authorization.AccountID == session.Authorization.AccountID && x.Spec.Config.Stacks[i] > 0 {
			return i, protocol.Verify(ReadDigest(x, r), r.Signature, session.Authorization.SessionKey)
		}
	}
	return -1, fmt.Errorf("not a hand participant")
}
func (s *Service) Recover(ctx context.Context, id common.Hash, r ReadRequest, after uint64, known common.Hash) (Recovery, error) {
	x, v, e := s.load(ctx, id)
	if e != nil {
		return Recovery{}, e
	}
	seat, e := s.readAuth(ctx, x, r)
	if e != nil {
		return Recovery{}, e
	}
	events, e := s.Store.Events(ctx, id, 0)
	if e != nil {
		return Recovery{}, e
	}
	if after > uint64(len(events)) {
		return Recovery{}, fmt.Errorf("unknown receipt cursor")
	}
	prev := common.Hash{}
	for i, event := range events {
		if event.HandID != id || event.Index != uint64(i) || event.Previous != prev || event.Verify() != nil {
			return Recovery{}, fmt.Errorf("receipt log integrity failure")
		}
		prev = event.Digest
	}
	if len(events) == 0 || events[len(events)-1].Index != v || prev != x.LastReceipt {
		return Recovery{}, fmt.Errorf("snapshot/log rollback or mismatch")
	}
	if after > 0 && events[after-1].Digest != known {
		return Recovery{}, fmt.Errorf("receipt fork detected")
	}
	p, e := public(x, v)
	if e != nil {
		return Recovery{}, e
	}
	out := Recovery{Public: p, Events: events[after:]}
	if p.Hand != nil {
		hole := p.Hand.Players[seat].Hole
		out.Hole = &hole
	}
	return out, nil
}
func (s *Service) Audit(ctx context.Context, id common.Hash, r ReadRequest) (*Snapshot, error) {
	x, _, e := s.load(ctx, id)
	if e != nil {
		return nil, e
	}
	if _, e = s.readAuth(ctx, x, r); e != nil {
		return nil, e
	}
	if x.Stage != "complete" {
		return nil, fmt.Errorf("seeds are disclosed only after completion")
	}
	return x, Verify(x)
}
func (s *Service) Timeout(ctx context.Context, id common.Hash) (Result, error) {
	return s.mutate(ctx, id, func(x *Snapshot, v uint64) (string, any, []common.Hash, *protocol.Receipt, error) {
		if x.Stage == "complete" || x.Stage == "aborted" {
			return "", nil, nil, nil, fmt.Errorf("hand already terminal")
		}
		if s.now() < x.Spec.ValidUntil {
			return "", nil, nil, nil, fmt.Errorf("deadline not reached")
		}
		stage := x.Stage
		x.Stage = "aborted"
		x.AbortReason = "hand deadline exceeded during " + stage
		return "abort", map[string]any{"reason": x.AbortReason, "deadline": x.Spec.ValidUntil}, nil, nil, nil
	})
}
func Verify(x *Snapshot) error {
	if x.Version != "signed-hand-v3" || x.Stage != "complete" || len(x.Moves) > int(x.Spec.ActionLimit) {
		return fmt.Errorf("unsupported or incomplete signed audit")
	}
	// Audit verifies cryptographic execution, not historical publication time.
	if x.Spec.ValidUntil == 0 {
		return fmt.Errorf("invalid deadline")
	}
	if e := protocol.ValidateSpec(x.Spec, x.Sessions, x.Spec.ValidUntil-1); e != nil {
		return e
	}
	ctx := x.Spec.Context()
	if protocol.SeedCommit(ctx, 255, x.ServerSeed) != x.ServerCommit {
		return fmt.Errorf("server seed mismatch")
	}
	manifest := protocol.Manifest(ctx, x.ServerCommit, x.Commits)
	if manifest != x.Manifest {
		return fmt.Errorf("manifest mismatch")
	}
	for i, stack := range x.Spec.Config.Stacks {
		if stack == 0 {
			if x.Commits[i] != (protocol.SignedValue{}) || x.Reveals[i] != (protocol.SignedValue{}) || x.Consents[i] != "" {
				return fmt.Errorf("data at vacant seat")
			}
			continue
		}
		sid := x.Spec.Sessions[i]
		key := x.Sessions[i].Authorization.SessionKey
		if protocol.SeedCommit(ctx, byte(i), x.Reveals[i].Value) != x.Commits[i].Value {
			return fmt.Errorf("player seed mismatch")
		}
		for _, check := range []struct {
			Digest common.Hash
			Sig    string
		}{{protocol.ValueDigest("COMMIT", sid, x.Spec.HandID, ctx, x.Commits[i].Value), x.Commits[i].Signature}, {protocol.ConsentDigest(sid, x.Spec.HandID, manifest), x.Consents[i]}, {protocol.ValueDigest("REVEAL", sid, x.Spec.HandID, manifest, x.Reveals[i].Value), x.Reveals[i].Signature}} {
			if e := protocol.Verify(check.Digest, check.Sig, key); e != nil {
				return e
			}
		}
	}
	deck, commit := protocol.Deck(ctx, x.ServerSeed, x.Reveals, occupied(x))
	if commit != x.DeckCommit {
		return fmt.Errorf("deck mismatch")
	}
	h, e := engine.New(x.Spec.Config, deck)
	if e != nil {
		return e
	}
	state := protocol.InitialState(ctx, manifest, commit)
	for i, m := range x.Moves {
		if m.Sequence != uint64(i+1) || m.Previous != state || m.Action.Seat < 0 || m.Action.Seat >= 9 {
			return fmt.Errorf("action sequence mismatch")
		}
		view, e := protocol.PublicView(h)
		if e != nil || view != m.PublicView || protocol.HoleCommit(ctx, m.Action.Seat, x.Reveals[m.Action.Seat].Value, h.Players[m.Action.Seat].Hole) != m.HoleCommit {
			return fmt.Errorf("observation mismatch")
		}
		digest, e := protocol.ActionDigest(x.Spec.Sessions[m.Action.Seat], x.Spec.HandID, state, m.Sequence, m.Action, m.PublicView, m.HoleCommit)
		if e != nil {
			return e
		}
		if e = protocol.Verify(digest, m.Signature, x.Sessions[m.Action.Seat].Authorization.SessionKey); e != nil {
			return e
		}
		if m.State != digest {
			return fmt.Errorf("action state mismatch")
		}
		if e = h.Apply(m.Action); e != nil {
			return e
		}
		state = digest
	}
	if state != x.State || h.Street != engine.Complete {
		return fmt.Errorf("incomplete action chain")
	}
	for i, p := range h.Players {
		if p.Stack != x.FinalStacks[i] {
			return fmt.Errorf("final chips mismatch")
		}
	}
	return nil
}

// Issue creates a certificate only after HTTP identity authentication. The
// client must prove possession of its locally generated key before registration.
func (s *Service) Issue(player common.Hash, kind uint8, domain protocol.Domain, key common.Address, publicKey string, tableID common.Hash, nonce, expiry uint64) (protocol.Session, error) {
	out := protocol.Session{Domain: domain, Authorization: protocol.Authorization{AccountID: player, Issuer: s.Address(), Kind: kind, SessionKey: key, TableID: tableID, Nonce: nonce, ExpiresAt: expiry}}
	if e := domain.Validate(); e != nil {
		return out, e
	}
	if player == (common.Hash{}) || key == (common.Address{}) || tableID == (common.Hash{}) || kind > 1 || expiry <= s.now() || expiry > s.now()+7*86400 {
		return out, fmt.Errorf("invalid certificate request")
	}
	raw, e := hex.DecodeString(strings.TrimPrefix(publicKey, "0x"))
	if e != nil {
		return out, e
	}
	pub, e := crypto.UnmarshalPubkey(raw)
	if e != nil || crypto.PubkeyToAddress(*pub) != key {
		return out, fmt.Errorf("invalid session public key")
	}
	out.Authorization.PlayerID = protocol.PlayerID(raw[1:])
	out.ID = out.Digest()
	sig, e := protocol.Sign(s.Key, out.ID)
	out.IssuerSignature = sig
	return out, e
}
