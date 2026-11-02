package engine

import (
	"fmt"
	"sort"
)

// Distribute constructs contribution tiers, includes folded dead money, and
// awards odd chips clockwise starting left of the button independently per pot.
func Distribute(contributions [Seats]int64, eligible [Seats]bool, ranks [Seats]Rank, button int) ([]Pot, [Seats]int64, error) {
	var payouts [Seats]int64
	if button < 0 || button >= Seats {
		return nil, payouts, fmt.Errorf("invalid button")
	}
	levels := []int64{}
	for _, c := range contributions {
		if c < 0 || c > MaxStack {
			return nil, payouts, fmt.Errorf("invalid contribution")
		}
		if c > 0 {
			levels = append(levels, c)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	pots := []Pot{}
	previous := int64(0)
	for _, level := range levels {
		if level == previous {
			continue
		}
		pot := Pot{}
		best := Rank(0)
		for i, c := range contributions {
			if c >= level {
				pot.Amount += level - previous
				if eligible[i] {
					pot.Eligible = append(pot.Eligible, i)
					best = max(best, ranks[i])
				}
			}
		}
		if len(pot.Eligible) == 0 {
			return nil, payouts, fmt.Errorf("pot without eligible player")
		}
		for j := 1; j <= Seats; j++ {
			i := (button + j) % Seats
			if contributions[i] >= level && eligible[i] && ranks[i] == best {
				pot.Winners = append(pot.Winners, i)
			}
		}
		share, remainder := pot.Amount/int64(len(pot.Winners)), pot.Amount%int64(len(pot.Winners))
		for j, i := range pot.Winners {
			payouts[i] += share
			if int64(j) < remainder {
				payouts[i]++
			}
		}
		pots = append(pots, pot)
		previous = level
	}
	return pots, payouts, nil
}
