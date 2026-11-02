package fairness

import "github.com/polygon-zk-poker/server/internal/engine"

// SimulateNine runs a nine-seat showdown with unequal all-ins and side pots.
// All entropy is generated in-process: this exercises replay, not a trustless ceremony.
func SimulateNine() (*Audit, *engine.Hand, error) {
	cfg := engine.Config{Stacks: [9]int64{1000, 700, 500, 1000, 350, 1000, 800, 1000, 600}, Button: 0, SmallBlind: 5, BigBlind: 10, Ante: 1}
	a, h, e := Start(cfg)
	if e != nil {
		return nil, nil, e
	}
	for h.Street != engine.Complete {
		kind := "check"
		if h.ToCall(h.Actor) > 0 {
			kind = "call"
		}
		if h.Street == engine.Preflop && h.CanRaise(h.Actor) {
			kind = "all-in"
		}
		if e := a.Append(h, engine.Action{Seat: h.Actor, Kind: kind}); e != nil {
			return nil, nil, e
		}
	}
	if e := a.Finish(h); e != nil {
		return nil, nil, e
	}
	return a, h, nil
}
