// Package fairness supplies reproducible commit/reveal and replay primitives.
// A replay is NOT a ZK proof or evidence that commitments were published on time.
package fairness

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/polygon-zk-poker/server/internal/engine"
)

const Version = "engine-audit-v1"
const MaxRecords = 4096

type Record struct {
	Action    engine.Action `json:"action"`
	StateHash string        `json:"stateHash"`
	Chain     string        `json:"chain"`
}

// Audit contains secrets and is intended for post-hand disclosure only.
type Audit struct {
	Version       string               `json:"version"`
	HandID        string               `json:"handId"`
	Config        engine.Config        `json:"config"`
	ServerCommit  string               `json:"serverCommit"`
	PlayerCommits [engine.Seats]string `json:"playerCommits"`
	DeckCommit    string               `json:"deckCommit"`
	ServerSeed    string               `json:"serverSeed"`
	PlayerSeeds   [engine.Seats]string `json:"playerSeeds"`
	InitialChain  string               `json:"initialChain"`
	Records       []Record             `json:"records"`
	FinalStacks   [engine.Seats]int64  `json:"finalStacks"`
}
type Report struct {
	HandID      string              `json:"handId"`
	Actions     int                 `json:"actions"`
	Chain       string              `json:"chain"`
	FinalStacks [engine.Seats]int64 `json:"finalStacks"`
	Scope       string              `json:"scope"`
}

func hash(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func decode32(s string) ([]byte, error) {
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 32 || s != strings.ToLower(s) {
		return nil, fmt.Errorf("expected canonical lowercase 32-byte hex")
	}
	return b, nil
}
func Context(handID string, c engine.Config) ([]byte, error) {
	id, e := decode32(handID)
	if e != nil {
		return nil, e
	}
	// Fixed-width encoding is language-independent and independent of JSON keys.
	b := append([]byte("zkpoker/context/v1\x00"), id...)
	for _, v := range c.Stacks {
		b = binary.BigEndian.AppendUint64(b, uint64(v))
	}
	b = binary.BigEndian.AppendUint64(b, uint64(c.Button))
	for _, v := range []int64{c.SmallBlind, c.BigBlind, c.Ante} {
		b = binary.BigEndian.AppendUint64(b, uint64(v))
	}
	d := sha256.Sum256(b)
	return d[:], nil
}
func Commitment(context []byte, seat int, seed []byte) string {
	// Server uses seat 255; players use 0..8. The role is domain separated.
	return hash([]byte("zkpoker/seed/v1\x00"), context, []byte{byte(seat)}, seed)
}

func (a *Audit) derive() ([52]engine.Card, error) {
	var deck [52]engine.Card
	ctx, e := Context(a.HandID, a.Config)
	if e != nil {
		return deck, e
	}
	server, e := decode32(a.ServerSeed)
	if e != nil {
		return deck, e
	}
	if Commitment(ctx, 255, server) != a.ServerCommit {
		return deck, fmt.Errorf("server commitment mismatch")
	}
	mix := append([]byte("zkpoker/mix/v1\x00"), ctx...)
	mix = append(mix, server...)
	for seat, stack := range a.Config.Stacks {
		if stack == 0 {
			if a.PlayerSeeds[seat] != "" || a.PlayerCommits[seat] != "" {
				return deck, fmt.Errorf("entropy at empty seat")
			}
			continue
		}
		seed, e := decode32(a.PlayerSeeds[seat])
		if e != nil {
			return deck, e
		}
		if Commitment(ctx, seat, seed) != a.PlayerCommits[seat] {
			return deck, fmt.Errorf("player %d commitment mismatch", seat)
		}
		mix = append(mix, byte(seat))
		mix = append(mix, seed...)
	}
	key := sha256.Sum256(mix)
	for i := range deck {
		deck[i] = engine.Card(i)
	}
	counter := uint64(0)
	for i := 51; i > 0; i-- {
		n := uint64(i + 1)
		threshold := -n % n
		var sample uint64
		for {
			mac := hmac.New(sha256.New, key[:])
			mac.Write([]byte("zkpoker/shuffle/v1\x00"))
			mac.Write(binary.BigEndian.AppendUint64(nil, counter))
			counter++
			sample = binary.BigEndian.Uint64(mac.Sum(nil)[:8])
			if sample >= threshold {
				break
			}
		}
		j := int(sample % n)
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck, nil
}
func deckCommit(ctx []byte, deck [52]engine.Card) string {
	b := make([]byte, 52)
	for i, c := range deck {
		b[i] = byte(c)
	}
	return hash([]byte("zkpoker/deck/v1\x00"), ctx, b)
}
func stateHash(h *engine.Hand) string {
	b, e := json.Marshal(h)
	if e != nil {
		panic(e)
	}
	return hash([]byte("zkpoker/state/json-v1\x00"), b)
}
func startChain(a *Audit, h *engine.Hand) string {
	ctx, _ := Context(a.HandID, a.Config)
	return hash([]byte("zkpoker/chain/v1\x00"), ctx, []byte(a.ServerCommit), []byte(strings.Join(a.PlayerCommits[:], "|")), []byte(a.DeckCommit), []byte(stateHash(h)))
}
func nextChain(previous string, sequence uint64, action engine.Action, state string) string {
	b := binary.BigEndian.AppendUint64(nil, sequence)
	b = append(b, byte(action.Seat))
	kinds := map[string]byte{"fold": 0, "check": 1, "call": 2, "bet": 3, "raise": 4, "all-in": 5}
	b = append(b, kinds[action.Kind])
	b = binary.BigEndian.AppendUint64(b, uint64(action.Amount))
	return hash([]byte("zkpoker/action/v1\x00"), []byte(previous), b, []byte(state))
}
func Start(cfg engine.Config) (*Audit, *engine.Hand, error) {
	a := &Audit{Version: Version, Config: cfg, Records: []Record{}}
	id := make([]byte, 32)
	if _, e := rand.Read(id); e != nil {
		return nil, nil, e
	}
	a.HandID = hex.EncodeToString(id)
	ctx, _ := Context(a.HandID, cfg)
	seed := make([]byte, 32)
	if _, e := rand.Read(seed); e != nil {
		return nil, nil, e
	}
	a.ServerSeed = hex.EncodeToString(seed)
	a.ServerCommit = Commitment(ctx, 255, seed)
	for i, s := range cfg.Stacks {
		if s > 0 {
			if _, e := rand.Read(seed); e != nil {
				return nil, nil, e
			}
			a.PlayerSeeds[i] = hex.EncodeToString(seed)
			a.PlayerCommits[i] = Commitment(ctx, i, seed)
		}
	}
	d, e := a.derive()
	if e != nil {
		return nil, nil, e
	}
	a.DeckCommit = deckCommit(ctx, d)
	h, e := engine.New(cfg, d)
	if e != nil {
		return nil, nil, e
	}
	a.InitialChain = startChain(a, h)
	return a, h, nil
}
func (a *Audit) Append(h *engine.Hand, action engine.Action) error {
	if len(a.Records) >= MaxRecords {
		return fmt.Errorf("audit action limit")
	}
	if e := h.Apply(action); e != nil {
		return e
	}
	previous := a.InitialChain
	if len(a.Records) > 0 {
		previous = a.Records[len(a.Records)-1].Chain
	}
	state := stateHash(h)
	a.Records = append(a.Records, Record{action, state, nextChain(previous, h.Sequence, action, state)})
	return nil
}
func (a *Audit) Finish(h *engine.Hand) error {
	if h.Street != engine.Complete {
		return fmt.Errorf("hand incomplete")
	}
	for i, p := range h.Players {
		a.FinalStacks[i] = p.Stack
	}
	return nil
}
func Verify(a *Audit) (*Report, error) {
	if a.Version != Version || len(a.Records) > MaxRecords {
		return nil, fmt.Errorf("unsupported audit or action limit")
	}
	d, e := a.derive()
	if e != nil {
		return nil, e
	}
	ctx, _ := Context(a.HandID, a.Config)
	if deckCommit(ctx, d) != a.DeckCommit {
		return nil, fmt.Errorf("deck commitment mismatch")
	}
	h, e := engine.New(a.Config, d)
	if e != nil {
		return nil, e
	}
	chain := startChain(a, h)
	if a.InitialChain != chain {
		return nil, fmt.Errorf("initial chain mismatch")
	}
	for i, r := range a.Records {
		if e := h.Apply(r.Action); e != nil {
			return nil, fmt.Errorf("action %d: %w", i, e)
		}
		state := stateHash(h)
		if state != r.StateHash {
			return nil, fmt.Errorf("state hash mismatch at action %d", i)
		}
		chain = nextChain(chain, h.Sequence, r.Action, state)
		if chain != r.Chain {
			return nil, fmt.Errorf("chain mismatch at action %d", i)
		}
	}
	if h.Street != engine.Complete {
		return nil, fmt.Errorf("hand incomplete")
	}
	for i, p := range h.Players {
		if p.Stack != a.FinalStacks[i] {
			return nil, fmt.Errorf("final stack mismatch at seat %d", i)
		}
	}
	return &Report{a.HandID, len(a.Records), chain, a.FinalStacks, "reproducible entropy and engine replay only; no signature, publication-time or ZK verification"}, nil
}
