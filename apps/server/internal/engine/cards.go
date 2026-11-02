package engine

import (
	"fmt"
	"sort"
	"strconv"
)

// Card is rank-major: 0=2c, 1=2d, ..., 51=As. Higher Rank wins.
type Card uint8
type Rank uint32

// Keep []Card a JSON number array: encoding/json otherwise treats a uint8 slice
// as bytes and emits a base64 string, breaking the protocol's card encoding.
func (c Card) MarshalJSON() ([]byte, error) {
	return strconv.AppendUint(nil, uint64(c), 10), nil
}

func (c Card) String() string {
	if c > 51 {
		return "??"
	}
	return string("23456789TJQKA"[c/4]) + string("cdhs"[c%4])
}

func ValidateDeck(deck [52]Card) error {
	var seen [52]bool
	for _, c := range deck {
		if c >= 52 || seen[c] {
			return fmt.Errorf("invalid or duplicate card: %d", c)
		}
		seen[c] = true
	}
	return nil
}

// Evaluate7 enumerates all 21 subsets; no external ranking table is trusted.
func Evaluate7(cards [7]Card) (Rank, error) {
	var seen [52]bool
	for _, c := range cards {
		if c >= 52 || seen[c] {
			return 0, fmt.Errorf("invalid or duplicate card: %d", c)
		}
		seen[c] = true
	}
	var best Rank
	for a := 0; a < 3; a++ {
		for b := a + 1; b < 4; b++ {
			for c := b + 1; c < 5; c++ {
				for d := c + 1; d < 6; d++ {
					for e := d + 1; e < 7; e++ {
						r := evaluate5([5]Card{cards[a], cards[b], cards[c], cards[d], cards[e]})
						if r > best {
							best = r
						}
					}
				}
			}
		}
	}
	return best, nil
}

func encode(category int, values ...int) Rank {
	r := Rank(category)
	for i := 0; i < 5; i++ {
		r *= 15
		if i < len(values) {
			r += Rank(values[i])
		}
	}
	return r
}

func evaluate5(cards [5]Card) Rank {
	var counts [15]int
	flush := true
	for _, c := range cards {
		counts[int(c/4)+2]++
		flush = flush && c%4 == cards[0]%4
	}
	ranks := make([]int, 0, 5)
	for r := 14; r >= 2; r-- {
		if counts[r] > 0 {
			ranks = append(ranks, r)
		}
	}
	straight := 0
	if len(ranks) == 5 {
		if ranks[0]-ranks[4] == 4 {
			straight = ranks[0]
		}
		if ranks[0] == 14 && ranks[1] == 5 && ranks[4] == 2 {
			straight = 5
		}
	}
	if flush && straight > 0 {
		return encode(8, straight)
	}
	type group struct{ count, rank int }
	groups := make([]group, 0, len(ranks))
	for _, r := range ranks {
		groups = append(groups, group{counts[r], r})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})
	if groups[0].count == 4 {
		return encode(7, groups[0].rank, groups[1].rank)
	}
	if groups[0].count == 3 && groups[1].count == 2 {
		return encode(6, groups[0].rank, groups[1].rank)
	}
	if flush {
		return encode(5, ranks...)
	}
	if straight > 0 {
		return encode(4, straight)
	}
	if groups[0].count == 3 {
		return encode(3, groups[0].rank, groups[1].rank, groups[2].rank)
	}
	if groups[0].count == 2 && groups[1].count == 2 {
		return encode(2, groups[0].rank, groups[1].rank, groups[2].rank)
	}
	if groups[0].count == 2 {
		return encode(1, groups[0].rank, groups[1].rank, groups[2].rank, groups[3].rank)
	}
	return encode(0, ranks...)
}
