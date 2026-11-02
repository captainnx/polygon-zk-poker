package protocol

import (
	"encoding/binary"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polygon-zk-poker/server/internal/engine"
)

func PublicView(h *engine.Hand) (common.Hash, error) {
	if h.Street == engine.Complete || h.Actor < 0 || h.Actor >= 9 || h.Sequence >= 64 {
		return common.Hash{}, fmt.Errorf("invalid action view")
	}
	b := []byte("ZKPVIEW3")
	append40 := func(v int64) error {
		if v < 0 || v > engine.MaxStack {
			return fmt.Errorf("view amount out of range")
		}
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(v))
		b = append(b, n[3:]...)
		return nil
	}
	var folded uint16
	for i, p := range h.Players {
		if e := append40(p.Stack); e != nil {
			return common.Hash{}, e
		}
		if e := append40(p.StreetBet); e != nil {
			return common.Hash{}, e
		}
		if p.Folded {
			folded |= 1 << i
		}
	}
	b = binary.BigEndian.AppendUint16(b, folded)
	if e := append40(h.CurrentBet); e != nil {
		return common.Hash{}, e
	}
	if e := append40(h.MinRaise); e != nil {
		return common.Hash{}, e
	}
	for i := 0; i < 5; i++ {
		if i < len(h.Board) {
			b = append(b, byte(h.Board[i]))
		} else {
			b = append(b, 0)
		}
	}
	streets := map[engine.Street]byte{engine.Preflop: 0, engine.Flop: 1, engine.Turn: 2, engine.River: 3}
	b = append(b, streets[h.Street], byte(h.Actor), byte(h.Sequence))
	return SHA(b), nil
}
func HoleCommit(context common.Hash, seat int, seed common.Hash, hole [2]engine.Card) common.Hash {
	return SHA([]byte("ZKPOKER_HOLE_V3"), context[:], []byte{byte(seat)}, seed[:], []byte{byte(hole[0]), byte(hole[1])})
}
