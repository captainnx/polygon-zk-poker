package main

import (
	"encoding/json"
	"fmt"
	"github.com/polygon-zk-poker/server/internal/engine"
	"math/rand"
	"os"
)

type action struct {
	Seat   int    `json:"seat"`
	Kind   uint8  `json:"kind"`
	Amount string `json:"amount"`
}
type vector struct {
	Config struct {
		Initial [9]string `json:"initial"`
		Button  int       `json:"button"`
		Small   string    `json:"small"`
		Big     string    `json:"big"`
		Ante    string    `json:"ante"`
	} `json:"config"`
	Deck    [52]engine.Card `json:"deck"`
	Count   int             `json:"action_count"`
	Actions [64]action      `json:"actions"`
	Final   [9]string       `json:"expected"`
}

func main() {
	rng := rand.New(rand.NewSource(912342))
	out := []vector{}
	kinds := map[string]uint8{"fold": 0, "check": 1, "call": 2, "bet": 3, "raise": 4, "all-in": 5}
	for round := 0; round < 40; round++ {
		n := 2 + round%8
		cfg := engine.Config{Button: round % n, SmallBlind: 5, BigBlind: 10, Ante: int64(round % 4)}
		for i := 0; i < n; i++ {
			cfg.Stacks[i] = int64(20 + rng.Intn(280))
		}
		var d [52]engine.Card
		for i := range d {
			d[i] = engine.Card(i)
		}
		rng.Shuffle(52, func(i, j int) { d[i], d[j] = d[j], d[i] })
		h, e := engine.New(cfg, d)
		if e != nil {
			panic(e)
		}
		v := vector{Deck: d}
		for i, s := range cfg.Stacks {
			v.Config.Initial[i] = fmt.Sprint(s)
		}
		v.Config.Button = cfg.Button
		v.Config.Small = "5"
		v.Config.Big = "10"
		v.Config.Ante = fmt.Sprint(cfg.Ante)
		for i := range v.Actions {
			v.Actions[i].Amount = "0"
		}
		for h.Street != engine.Complete {
			a := engine.Action{Seat: h.Actor, Kind: "check"}
			if h.ToCall(h.Actor) > 0 {
				a.Kind = "call"
			}
			if v.Count < 18 {
				choice := rng.Intn(7)
				if choice == 0 {
					a.Kind = "fold"
				}
				if choice == 1 && h.CanRaise(h.Actor) {
					a.Kind = "all-in"
				}
				if choice == 2 && h.CanRaise(h.Actor) {
					p := h.Players[h.Actor]
					minimum := h.CurrentBet + h.MinRaise
					if minimum <= p.Stack+p.StreetBet {
						a.Kind = "raise"
						if h.CurrentBet == 0 {
							a.Kind = "bet"
						}
						a.Amount = minimum
					}
				}
			}
			if e = h.Apply(a); e != nil {
				panic(e)
			}
			if v.Count >= 64 {
				panic("vector capacity")
			}
			v.Actions[v.Count] = action{a.Seat, kinds[a.Kind], fmt.Sprint(a.Amount)}
			v.Count++
		}
		for i, p := range h.Players {
			v.Final[i] = fmt.Sprint(p.Stack)
		}
		out = append(out, v)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if e := enc.Encode(out); e != nil {
		panic(e)
	}
}
