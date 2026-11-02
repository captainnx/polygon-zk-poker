// Package engine implements deterministic, single-hand no-limit Hold'em.
// A Hand belongs to one actor/goroutine. Network callers must serialize actions.
package engine

import "fmt"

const Seats = 9
const MaxStack int64 = 1_000_000_000_000

type Street string

const (
	Preflop  Street = "preflop"
	Flop     Street = "flop"
	Turn     Street = "turn"
	River    Street = "river"
	Complete Street = "complete"
)

type Config struct {
	Stacks     [Seats]int64 `json:"stacks"`
	Button     int          `json:"button"`
	SmallBlind int64        `json:"smallBlind"`
	BigBlind   int64        `json:"bigBlind"`
	Ante       int64        `json:"ante"`
}
type Player struct {
	Occupied  bool    `json:"occupied"`
	Stack     int64   `json:"stack"`
	Committed int64   `json:"committed"`
	StreetBet int64   `json:"streetBet"`
	Folded    bool    `json:"folded"`
	Hole      [2]Card `json:"-"`
	acted     bool
	lastBet   int64
}
type Action struct {
	Seat int    `json:"seat"`
	Kind string `json:"kind"`
	// Amount is the total street contribution for bet/raise, not the increment.
	Amount int64 `json:"amount"`
}
type Pot struct {
	Amount   int64 `json:"amount"`
	Eligible []int `json:"eligible"`
	Winners  []int `json:"winners"`
}
type Hand struct {
	Config     Config        `json:"config"`
	Players    [Seats]Player `json:"players"`
	Street     Street        `json:"street"`
	Actor      int           `json:"actor"`
	CurrentBet int64         `json:"currentBet"`
	MinRaise   int64         `json:"minRaise"`
	Board      []Card        `json:"board"`
	Pots       []Pot         `json:"pots,omitempty"`
	Payouts    [Seats]int64  `json:"payouts"`
	Refunds    [Seats]int64  `json:"refunds"`
	Sequence   uint64        `json:"sequence"`
	deck       [52]Card
	cursor     int
	total      int64
}

func New(cfg Config, deck [52]Card) (*Hand, error) {
	if cfg.SmallBlind <= 0 || cfg.BigBlind < cfg.SmallBlind || cfg.BigBlind > MaxStack || cfg.Ante < 0 || cfg.Ante > MaxStack {
		return nil, fmt.Errorf("invalid blinds or ante")
	}
	if err := ValidateDeck(deck); err != nil {
		return nil, err
	}
	h := &Hand{Config: cfg, Street: Preflop, Actor: -1, CurrentBet: cfg.BigBlind, MinRaise: cfg.BigBlind, deck: deck}
	n := 0
	for i, s := range cfg.Stacks {
		if s < 0 || s > MaxStack {
			return nil, fmt.Errorf("invalid stack at seat %d", i)
		}
		if s > 0 {
			n++
			h.Players[i] = Player{Occupied: true, Stack: s}
			h.total += s
		}
	}
	if n < 2 || cfg.Button < 0 || cfg.Button >= Seats || !h.Players[cfg.Button].Occupied {
		return nil, fmt.Errorf("need 2–9 players and an occupied button")
	}
	for i := range h.Players {
		if h.Players[i].Occupied {
			h.pay(i, min(cfg.Ante, h.Players[i].Stack), false)
		}
	}
	sb := h.next(cfg.Button, func(p Player) bool { return p.Occupied })
	if n == 2 {
		sb = cfg.Button
	}
	bb := h.next(sb, func(p Player) bool { return p.Occupied })
	h.pay(sb, min(cfg.SmallBlind, h.Players[sb].Stack), true)
	h.pay(bb, min(cfg.BigBlind, h.Players[bb].Stack), true)
	for round := 0; round < 2; round++ {
		seat := cfg.Button
		for j := 0; j < n; j++ {
			seat = h.next(seat, func(p Player) bool { return p.Occupied })
			h.Players[seat].Hole[round] = h.draw()
		}
	}
	h.advance(bb)
	return h, h.CheckInvariant()
}

func (h *Hand) next(from int, pred func(Player) bool) int {
	for j := 1; j <= Seats; j++ {
		s := (from + j + Seats) % Seats
		if pred(h.Players[s]) {
			return s
		}
	}
	return -1
}
func live(p Player) bool   { return p.Occupied && !p.Folded }
func canAct(p Player) bool { return live(p) && p.Stack > 0 }
func (h *Hand) count(pred func(Player) bool) int {
	n := 0
	for _, p := range h.Players {
		if pred(p) {
			n++
		}
	}
	return n
}
func (h *Hand) draw() Card { c := h.deck[h.cursor]; h.cursor++; return c }
func (h *Hand) pay(seat int, amount int64, street bool) {
	p := &h.Players[seat]
	p.Stack -= amount
	p.Committed += amount
	if street {
		p.StreetBet += amount
	}
}
func (h *Hand) ToCall(seat int) int64 {
	if seat < 0 || seat >= Seats || !canAct(h.Players[seat]) {
		return 0
	}
	return min(max(0, h.CurrentBet-h.Players[seat].StreetBet), h.Players[seat].Stack)
}
func (h *Hand) CanRaise(seat int) bool {
	if seat < 0 || seat >= Seats || !canAct(h.Players[seat]) || h.count(canAct) < 2 {
		return false
	}
	p := h.Players[seat]
	return p.Stack+p.StreetBet > h.CurrentBet && (!p.acted || h.CurrentBet-p.lastBet >= h.MinRaise)
}

// Apply is transactional: rejected actions leave the entire hand unchanged.
func (h *Hand) Apply(a Action) error {
	copy := *h
	copy.Board = append([]Card(nil), h.Board...)
	if err := copy.apply(a); err != nil {
		return err
	}
	if err := copy.CheckInvariant(); err != nil {
		return err
	}
	*h = copy
	return nil
}
func (h *Hand) apply(a Action) error {
	if h.Street == Complete || a.Seat < 0 || a.Seat >= Seats || a.Seat != h.Actor || !canAct(h.Players[a.Seat]) {
		return fmt.Errorf("not this seat's turn")
	}
	p := &h.Players[a.Seat]
	if a.Amount < 0 || a.Amount > MaxStack {
		return fmt.Errorf("invalid amount")
	}
	if a.Kind != "bet" && a.Kind != "raise" && a.Amount != 0 {
		return fmt.Errorf("amount only valid for bet/raise")
	}
	kind, target := a.Kind, a.Amount
	if kind == "all-in" {
		target = p.StreetBet + p.Stack
		if target <= h.CurrentBet {
			kind = "call"
		} else if h.CurrentBet == 0 {
			kind = "bet"
		} else {
			kind = "raise"
		}
	}
	switch kind {
	case "fold":
		p.Folded = true
	case "check":
		if p.StreetBet < h.CurrentBet {
			return fmt.Errorf("cannot check facing a bet")
		}
	case "call":
		if p.StreetBet >= h.CurrentBet {
			return fmt.Errorf("nothing to call")
		}
		h.pay(a.Seat, h.ToCall(a.Seat), true)
	case "bet", "raise":
		if (kind == "bet") != (h.CurrentBet == 0) {
			return fmt.Errorf("use bet to open; raise facing a bet")
		}
		if !h.CanRaise(a.Seat) {
			return fmt.Errorf("betting is not reopened or no opponent can respond")
		}
		if target <= h.CurrentBet || target > p.StreetBet+p.Stack {
			return fmt.Errorf("invalid raise target")
		}
		increase := target - h.CurrentBet
		if increase < h.MinRaise && target != p.StreetBet+p.Stack {
			return fmt.Errorf("short raise must be all-in")
		}
		if increase >= h.MinRaise {
			h.MinRaise = increase
		}
		h.pay(a.Seat, target-p.StreetBet, true)
		h.CurrentBet = target
	default:
		return fmt.Errorf("unknown action %q", a.Kind)
	}
	p.acted = true
	p.lastBet = h.CurrentBet
	h.Sequence++
	h.advance(a.Seat)
	return nil
}

func (h *Hand) advance(from int) {
	for {
		if h.count(live) == 1 {
			h.settle()
			return
		}
		n := h.count(canAct)
		if n == 1 {
			// There is no dry-side-pot betting. Only a call/fold decision remains.
			sole := h.next(-1, canAct)
			var otherMax int64
			for i, p := range h.Players {
				if i != sole {
					otherMax = max(otherMax, p.StreetBet)
				}
			}
			h.CurrentBet = otherMax
			if h.Players[sole].StreetBet < otherMax {
				h.Actor = sole
				return
			}
		}
		if n >= 2 {
			s := h.next(from, func(p Player) bool { return canAct(p) && (!p.acted || p.StreetBet < h.CurrentBet) })
			if s >= 0 {
				h.Actor = s
				return
			}
		}
		h.refundUncalled()
		if h.Street == River {
			h.settle()
			return
		}
		h.draw() // burn before each community street
		switch h.Street {
		case Preflop:
			h.Street = Flop
			h.Board = append(h.Board, h.draw(), h.draw(), h.draw())
		case Flop:
			h.Street = Turn
			h.Board = append(h.Board, h.draw())
		case Turn:
			h.Street = River
			h.Board = append(h.Board, h.draw())
		}
		h.CurrentBet = 0
		h.MinRaise = h.Config.BigBlind
		for i := range h.Players {
			h.Players[i].StreetBet = 0
			h.Players[i].acted = false
			h.Players[i].lastBet = 0
		}
		from = h.Config.Button
	}
}
func (h *Hand) refundUncalled() {
	first, second, seat := int64(0), int64(0), -1
	for i, p := range h.Players {
		if p.Committed > first {
			second = first
			first = p.Committed
			seat = i
		} else if p.Committed > second {
			second = p.Committed
		}
	}
	if seat >= 0 && first > second {
		d := first - second
		p := &h.Players[seat]
		p.Committed -= d
		p.Stack += d
		p.StreetBet -= min(d, p.StreetBet)
		h.Refunds[seat] += d
	}
}

func (h *Hand) settle() {
	h.refundUncalled()
	var ranks [Seats]Rank
	if h.count(live) > 1 {
		for i, p := range h.Players {
			if live(p) {
				cards := [7]Card{p.Hole[0], p.Hole[1], h.Board[0], h.Board[1], h.Board[2], h.Board[3], h.Board[4]}
				r, err := Evaluate7(cards)
				if err != nil {
					panic(err)
				}
				ranks[i] = r
			}
		}
	}
	contributions := [Seats]int64{}
	eligible := [Seats]bool{}
	for i, p := range h.Players {
		contributions[i] = p.Committed
		eligible[i] = live(p)
	}
	pots, payouts, err := Distribute(contributions, eligible, ranks, h.Config.Button)
	if err != nil {
		panic(err)
	} // only internal invariant violations can reach this
	h.Pots = pots
	h.Payouts = payouts
	for i := range h.Players {
		h.Players[i].Stack += payouts[i]
		h.Players[i].Committed = 0
		h.Players[i].StreetBet = 0
	}
	h.Street = Complete
	h.Actor = -1
}

func (h *Hand) CheckInvariant() error {
	var sum int64
	for i, p := range h.Players {
		if p.Stack < 0 || p.Committed < 0 || p.StreetBet < 0 || p.StreetBet > p.Committed {
			return fmt.Errorf("invalid chips at seat %d", i)
		}
		if !p.Occupied && (p.Stack != 0 || p.Committed != 0) {
			return fmt.Errorf("chips at empty seat")
		}
		sum += p.Stack + p.Committed
	}
	if sum != h.total {
		return fmt.Errorf("chip conservation: got %d want %d", sum, h.total)
	}
	if h.Street != Complete && (h.Actor < 0 || h.Actor >= Seats || !canAct(h.Players[h.Actor])) {
		return fmt.Errorf("invalid actor")
	}
	return nil
}
