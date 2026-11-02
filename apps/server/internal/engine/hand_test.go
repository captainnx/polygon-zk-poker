package engine

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
)

func deck() [52]Card {
	var d [52]Card
	for i := range d {
		d[i] = Card(i)
	}
	return d
}
func config(stacks ...int64) Config {
	c := Config{Button: 0, SmallBlind: 5, BigBlind: 10}
	copy(c.Stacks[:], stacks)
	return c
}
func mustNew(t testing.TB, c Config) *Hand {
	t.Helper()
	h, e := New(c, deck())
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func act(t testing.TB, h *Hand, kind string, amount int64) {
	t.Helper()
	if e := h.Apply(Action{Seat: h.Actor, Kind: kind, Amount: amount}); e != nil {
		t.Fatalf("%s %d at %s seat %d: %v", kind, amount, h.Street, h.Actor, e)
	}
}
func finish(t testing.TB, h *Hand) {
	t.Helper()
	for n := 0; h.Street != Complete; n++ {
		if n > 100 {
			t.Fatal("did not finish")
		}
		kind := "check"
		if h.ToCall(h.Actor) > 0 {
			kind = "call"
		}
		act(t, h, kind, 0)
	}
}

func TestNineHandAndBlinds(t *testing.T) {
	c := config(1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000)
	c.Ante = 2
	h := mustNew(t, c)
	if h.Actor != 3 || h.Players[1].Committed != 7 || h.Players[2].Committed != 12 {
		t.Fatal(h)
	}
	var seen [52]bool
	for _, p := range h.Players {
		for _, c := range p.Hole {
			if seen[c] {
				t.Fatal("duplicate deal")
			}
			seen[c] = true
		}
	}
	finish(t, h)
	if len(h.Board) != 5 || h.Sequence != 36 || h.cursor != 26 {
		t.Fatalf("board=%v seq=%d cursor=%d", h.Board, h.Sequence, h.cursor)
	}
	if h.Pots[0].Amount != 108 {
		t.Fatal(h.Pots)
	}
}
func TestHeadsUpOrder(t *testing.T) {
	c := config(100, 0, 0, 0, 0, 0, 0, 100)
	h := mustNew(t, c)
	if h.Actor != 0 || h.Players[0].StreetBet != 5 || h.Players[7].StreetBet != 10 {
		t.Fatal(h)
	}
	act(t, h, "call", 0)
	act(t, h, "check", 0)
	if h.Street != Flop || h.Actor != 7 {
		t.Fatal("HU postflop order")
	}
	finish(t, h)
}
func TestBigBlindOption(t *testing.T) {
	h := mustNew(t, config(100, 100, 100))
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	if h.Actor != 2 || h.Street != Preflop {
		t.Fatal("BB lost option")
	}
	act(t, h, "raise", 30)
	if h.MinRaise != 20 || h.Actor != 0 {
		t.Fatal(h)
	}
}
func TestShortRaiseDoesNotReopen(t *testing.T) {
	h := mustNew(t, config(100, 15, 100))
	act(t, h, "call", 0)
	act(t, h, "all-in", 0)
	act(t, h, "call", 0)
	if h.Actor != 0 || h.CanRaise(0) {
		t.Fatal("short raise reopened")
	}
	before := *h
	if h.Apply(Action{Seat: 0, Kind: "raise", Amount: 30}) == nil || !reflect.DeepEqual(before, *h) {
		t.Fatal("must reject atomically")
	}
	act(t, h, "call", 0)
	finish(t, h)
}
func TestCumulativeShortRaisesReopen(t *testing.T) {
	h := mustNew(t, config(100, 15, 20, 100)) // seat 3 calls 10, then button calls
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	act(t, h, "all-in", 0)
	act(t, h, "all-in", 0)
	if h.Actor != 3 || !h.CanRaise(3) || h.MinRaise != 10 {
		t.Fatal("cumulative full raise not reopened")
	}
	act(t, h, "raise", 30)
	finish(t, h)
}
func TestFullRaiseReopensAndSetsMinimum(t *testing.T) {
	h := mustNew(t, config(200, 200, 200))
	act(t, h, "raise", 30)
	act(t, h, "raise", 70)
	act(t, h, "call", 0)
	if h.Actor != 0 || !h.CanRaise(0) || h.MinRaise != 40 {
		t.Fatal(h)
	}
	if h.Apply(Action{Seat: 0, Kind: "raise", Amount: 100}) == nil {
		t.Fatal("underraise accepted")
	}
	act(t, h, "raise", 110)
	finish(t, h)
}
func TestShortOpeningAllIn(t *testing.T) {
	h := mustNew(t, config(100, 13, 100))
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	act(t, h, "check", 0)
	act(t, h, "all-in", 0) // SB has 3 left, opens flop below minimum
	if h.CurrentBet != 3 || h.MinRaise != 10 {
		t.Fatal(h)
	}
	if h.Apply(Action{Seat: h.Actor, Kind: "raise", Amount: 10}) == nil {
		t.Fatal("cannot complete a short bet to the minimum")
	}
	act(t, h, "raise", 13)
	finish(t, h)
}

func TestOpeningBetAndStreetReset(t *testing.T) {
	h := mustNew(t, config(200, 200, 200))
	act(t, h, "raise", 40)
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	if h.Street != Flop || h.MinRaise != 10 || h.CurrentBet != 0 {
		t.Fatal("street did not reset")
	}
	if h.Apply(Action{Seat: h.Actor, Kind: "bet", Amount: 9}) == nil {
		t.Fatal("under-minimum opening bet accepted")
	}
	act(t, h, "bet", 20)
	act(t, h, "raise", 40)
	finish(t, h)
}
func TestCheckBeforeShortBetDoesNotReopen(t *testing.T) {
	h := mustNew(t, config(13, 100, 100))
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	act(t, h, "check", 0)
	act(t, h, "check", 0)
	act(t, h, "check", 0)
	act(t, h, "all-in", 0)
	if h.Actor != 1 || h.CanRaise(1) {
		t.Fatal("short bet reopened checking player under this ruleset")
	}
	finish(t, h)
}
func TestAllInRunoutAndUncalledRefund(t *testing.T) {
	h := mustNew(t, config(100, 30, 60))
	act(t, h, "all-in", 0)
	act(t, h, "all-in", 0)
	act(t, h, "all-in", 0)
	if h.Street != Complete || len(h.Board) != 5 || h.Refunds[0] != 40 {
		t.Fatal(h)
	}
	if len(h.Pots) != 2 || h.Pots[0].Amount != 90 || h.Pots[1].Amount != 60 {
		t.Fatal(h.Pots)
	}
}
func TestShortCallCannotCreateDrySidePot(t *testing.T) {
	h := mustNew(t, config(100, 7))
	act(t, h, "call", 0)
	if h.Street != Complete || h.Refunds[0] != 0 {
		t.Fatal(h)
	}
	if h.Pots[0].Amount != 14 {
		t.Fatal(h.Pots)
	}
}
func TestShortBlindBringIn(t *testing.T) {
	h := mustNew(t, config(100, 100, 3))
	if h.ToCall(0) != 10 {
		t.Fatal("nominal big blind required multiway")
	}
	act(t, h, "call", 0)
	act(t, h, "call", 0)
	finish(t, h)
}
func TestAnteExhaustsPlayers(t *testing.T) {
	c := config(3, 4, 5)
	c.Ante = 10
	h := mustNew(t, c)
	if h.Street != Complete || h.Refunds[2] != 1 {
		t.Fatal(h)
	}
}
func TestFoldWin(t *testing.T) {
	h := mustNew(t, config(100, 100, 100))
	act(t, h, "fold", 0)
	act(t, h, "fold", 0)
	if h.Street != Complete || len(h.Board) != 0 || h.Players[2].Stack != 105 || h.Refunds[2] != 5 {
		t.Fatal(h)
	}
	if h.Apply(Action{Seat: 2, Kind: "check"}) == nil {
		t.Fatal("completed hand accepts action")
	}
}
func TestFoldedMoneyAndOddChips(t *testing.T) {
	contrib := [Seats]int64{5, 5, 5, 10, 10}
	eligible := [Seats]bool{true, true, false, true, true}
	ranks := [Seats]Rank{9, 9, 99, 4, 4}
	pots, pay, e := Distribute(contrib, eligible, ranks, 0)
	if e != nil {
		t.Fatal(e)
	}
	if len(pots) != 2 || pay != ([Seats]int64{12, 13, 0, 5, 5}) {
		t.Fatal(pots, pay)
	}
}

func TestBoardTieReturnsEqualStacks(t *testing.T) {
	d := deck()
	// Heads-up: four hole cards, then burns at 4/8/10. Put a royal flush
	// on the board so both players must use the same best five cards.
	for position, card := range map[int]Card{5: 32, 6: 36, 7: 40, 9: 44, 11: 48} {
		for i, c := range d {
			if c == card {
				d[position], d[i] = d[i], d[position]
				break
			}
		}
	}
	h, err := New(config(100, 100), d)
	if err != nil {
		t.Fatal(err)
	}
	finish(t, h)
	if h.Players[0].Stack != 100 || h.Players[1].Stack != 100 || len(h.Pots[0].Winners) != 2 {
		t.Fatal(h.Pots, h.Players)
	}
}
func TestSidePotWinners(t *testing.T) {
	contrib := [Seats]int64{100, 200, 300, 300}
	eligible := [Seats]bool{true, true, true, false}
	ranks := [Seats]Rank{9, 8, 7, 99}
	pots, pay, e := Distribute(contrib, eligible, ranks, 3)
	if e != nil {
		t.Fatal(e)
	}
	if len(pots) != 3 || pay != ([Seats]int64{400, 300, 200}) {
		t.Fatal(pots, pay)
	}
}
func TestInvalidActionsAtomic(t *testing.T) {
	for _, a := range []Action{{0, "check", 0}, {1, "call", 0}, {0, "call", 1}, {0, "raise", 15}, {0, "raise", 101}, {0, "bet", 20}, {0, "nope", 0}, {0, "raise", -1}, {-1, "call", 0}, {9, "call", 0}, {0, "all-in", 1}} {
		h := mustNew(t, config(100, 100, 100))
		before := *h
		if h.Apply(a) == nil || !reflect.DeepEqual(before, *h) {
			t.Fatalf("action %+v not atomic", a)
		}
	}
}
func TestInvalidConfiguration(t *testing.T) {
	for _, c := range []Config{config(100), config(-1, 100), config(MaxStack+1, 100), {Stacks: [Seats]int64{100, 100}, Button: 8, SmallBlind: 5, BigBlind: 10}, {Stacks: [Seats]int64{100, 100}, SmallBlind: 10, BigBlind: 5}} {
		if _, e := New(c, deck()); e == nil {
			t.Fatal(c)
		}
	}
	d := deck()
	d[51] = 0
	if _, e := New(config(100, 100), d); e == nil {
		t.Fatal("duplicate deck")
	}
	d = deck()
	d[51] = 52
	if _, e := New(config(100, 100), d); e == nil {
		t.Fatal("out-of-range deck")
	}
}
func TestNoSecretCardsInJSON(t *testing.T) {
	h := mustNew(t, config(100, 100))
	data, _ := json.Marshal(h)
	var obj map[string]any
	if e := json.Unmarshal(data, &obj); e != nil {
		t.Fatal(e)
	}
	if _, ok := obj["deck"]; ok {
		t.Fatal("deck exposed")
	}
	for _, v := range obj["players"].([]any) {
		if _, ok := v.(map[string]any)["Hole"]; ok {
			t.Fatal("hole cards exposed")
		}
	}
}

func TestCommunityCardsAreJSONNumbers(t *testing.T) {
	h := mustNew(t, config(100, 100))
	finish(t, h)
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatal(err)
	}
	board, ok := obj["board"].([]any)
	if !ok || len(board) != 5 {
		t.Fatalf("board must be array: %s", data)
	}
	for _, card := range board {
		if _, ok := card.(float64); !ok {
			t.Fatal("non-numeric card")
		}
	}
}
func FuzzHandConservation(f *testing.F) {
	f.Add([]byte{9, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{2, 0, 0})
	f.Add([]byte{3, 255, 0, 5})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		rng := rand.New(rand.NewSource(int64(data[0])))
		d := deck()
		rng.Shuffle(52, func(i, j int) { d[i], d[j] = d[j], d[i] })
		n := 2 + int(data[0])%8
		c := config()
		for i := 0; i < n; i++ {
			c.Stacks[i] = 1 + int64(data[i%len(data)])
		}
		c.Ante = int64(data[len(data)-1] % 4)
		h, e := New(c, d)
		if e != nil {
			t.Fatal(e)
		}
		for step := 0; h.Street != Complete; step++ {
			if step > 600 {
				t.Fatal("nonterminating")
			}
			b := data[step%len(data)]
			kind := "check"
			amount := int64(0)
			if h.ToCall(h.Actor) > 0 {
				kind = "call"
			}
			if b%5 == 0 && step < 100 {
				kind = "fold"
			}
			if b%5 == 1 && step < 100 && h.CanRaise(h.Actor) {
				kind = "all-in"
			}
			if b%5 == 2 && step < 100 && h.CanRaise(h.Actor) {
				p := h.Players[h.Actor]
				minimum := h.CurrentBet + h.MinRaise
				maximum := p.StreetBet + p.Stack
				if maximum >= minimum {
					kind = "raise"
					if h.CurrentBet == 0 {
						kind = "bet"
					}
					amount = minimum + int64(b)%(maximum-minimum+1)
				}
			}
			act(t, h, kind, amount)
			if e := h.CheckInvariant(); e != nil {
				t.Fatal(e)
			}
		}
	})
}
func FuzzRejectedActionsAtomic(f *testing.F) {
	f.Add(uint8(0), int64(11), "raise")
	f.Add(uint8(9), int64(-1), "call")
	f.Fuzz(func(t *testing.T, seat uint8, amount int64, kind string) {
		h := mustNew(t, config(100, 100, 100))
		before := *h
		if e := h.Apply(Action{int(seat), kind, amount}); e != nil && !reflect.DeepEqual(before, *h) {
			t.Fatal("mutation after rejection")
		}
	})
}
func BenchmarkNinePlayerHand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		h := mustNew(b, config(1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000))
		finish(b, h)
	}
}
