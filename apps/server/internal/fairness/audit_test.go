package fairness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/polygon-zk-poker/server/internal/engine"
)

func TestNineReplay(t *testing.T) {
	a, h, e := SimulateNine()
	if e != nil {
		t.Fatal(e)
	}
	r, e := Verify(a)
	if e != nil {
		t.Fatal(e)
	}
	if r.Actions != 9 || len(h.Pots) < 2 {
		t.Fatal(r, h.Pots)
	}
}
func TestTamperedAudit(t *testing.T) {
	a, _, e := SimulateNine()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(a)
	cases := map[string]func(*Audit){
		"seed":              func(x *Audit) { x.ServerSeed = strings.Repeat("00", 32) },
		"player seed":       func(x *Audit) { x.PlayerSeeds[3] = strings.Repeat("00", 32) },
		"deck order":        func(x *Audit) { x.DeckCommit = strings.Repeat("00", 32) },
		"illegal bet":       func(x *Audit) { x.Records[0].Action = engine.Action{Seat: 3, Kind: "raise", Amount: 11} },
		"replay other hand": func(x *Audit) { x.HandID = strings.Repeat("00", 32) },
		"stack":             func(x *Audit) { x.FinalStacks[0]++ },
		"state and pots":    func(x *Audit) { x.Records[8].StateHash = strings.Repeat("00", 32) },
		"chain":             func(x *Audit) { x.Records[0].Chain = strings.Repeat("00", 32) },
		"truncate":          func(x *Audit) { x.Records = x.Records[:8] },
		"duplicate action":  func(x *Audit) { x.Records = append(x.Records, x.Records[8]) },
		"rules":             func(x *Audit) { x.Config.Ante++ },
		"version":           func(x *Audit) { x.Version = "unknown" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var x Audit
			if e := json.Unmarshal(raw, &x); e != nil {
				t.Fatal(e)
			}
			change(&x)
			if _, e := Verify(&x); e == nil {
				t.Fatal("tamper accepted")
			}
		})
	}
}
func TestCommitBindingAndShuffle(t *testing.T) {
	a, _, e := Start(engine.Config{Stacks: [9]int64{100, 100}, SmallBlind: 5, BigBlind: 10})
	if e != nil {
		t.Fatal(e)
	}
	x, e := a.derive()
	if e != nil {
		t.Fatal(e)
	}
	y, e := a.derive()
	if e != nil {
		t.Fatal(e)
	}
	if x != y {
		t.Fatal("nondeterministic")
	}
	if e := engine.ValidateDeck(x); e != nil {
		t.Fatal(e)
	}
	ctx, _ := Context(a.HandID, a.Config)
	seed, _ := decode32(a.PlayerSeeds[0])
	if Commitment(ctx, 0, seed) == Commitment(ctx, 1, seed) || Commitment(ctx, 0, seed) == Commitment(ctx, 255, seed) {
		t.Fatal("role collision")
	}
	a.PlayerCommits[8] = a.PlayerCommits[0]
	if _, e := a.derive(); e == nil {
		t.Fatal("empty seat commitment")
	}
}
