package engine

import "testing"

func TestSevenCardCategories(t *testing.T) {
	cases := []struct {
		name     string
		cards    [7]Card
		category int
	}{
		{"straight flush", [7]Card{32, 36, 40, 44, 48, 1, 6}, 8},
		{"quads", [7]Card{48, 49, 50, 51, 44, 1, 6}, 7},
		{"two trips full house", [7]Card{48, 49, 50, 44, 45, 46, 1}, 6},
		{"flush", [7]Card{48, 40, 28, 16, 4, 1, 6}, 5},
		{"wheel", [7]Card{48, 1, 6, 11, 12, 22, 31}, 4},
		{"trips", [7]Card{48, 49, 50, 40, 33, 22, 11}, 3},
		{"two pair", [7]Card{48, 49, 44, 45, 36, 21, 10}, 2},
		{"pair", [7]Card{48, 49, 40, 33, 26, 17, 6}, 1},
		{"high", [7]Card{48, 41, 34, 23, 16, 9, 2}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, e := Evaluate7(tc.cards)
			if e != nil {
				t.Fatal(e)
			}
			if int(r)/759375 != tc.category {
				t.Fatal(r)
			}
		})
	}
}
func TestKickersAndWheel(t *testing.T) {
	if evaluate5([5]Card{48, 49, 50, 51, 44}) <= evaluate5([5]Card{48, 49, 50, 51, 40}) {
		t.Fatal("quad kicker")
	}
	if evaluate5([5]Card{0, 5, 10, 15, 16}) <= evaluate5([5]Card{48, 1, 6, 11, 12}) {
		t.Fatal("6 high must beat wheel")
	}
	a, _ := Evaluate7([7]Card{0, 5, 32, 36, 40, 44, 48})
	b, _ := Evaluate7([7]Card{1, 6, 32, 36, 40, 44, 48})
	if a != b {
		t.Fatal("board tie")
	}
	if _, e := Evaluate7([7]Card{0, 0, 1, 2, 3, 4, 5}); e == nil {
		t.Fatal("duplicate")
	}
	if _, e := Evaluate7([7]Card{52, 0, 1, 2, 3, 4, 5}); e == nil {
		t.Fatal("range")
	}
}

// Independent known combinatorial frequencies over all C(52,5) hands.
func TestFiveCardCategoryFrequencies(t *testing.T) {
	if testing.Short() {
		t.Skip("exhaustive 2,598,960-hand classification")
	}
	var counts [9]int
	for a := 0; a < 48; a++ {
		for b := a + 1; b < 49; b++ {
			for c := b + 1; c < 50; c++ {
				for d := c + 1; d < 51; d++ {
					for e := d + 1; e < 52; e++ {
						r := evaluate5([5]Card{Card(a), Card(b), Card(c), Card(d), Card(e)})
						counts[int(r)/759375]++
					}
				}
			}
		}
	}
	want := [9]int{1302540, 1098240, 123552, 54912, 10200, 5108, 3744, 624, 40}
	if counts != want {
		t.Fatalf("got %v want %v", counts, want)
	}
}
