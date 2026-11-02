package protocol

import (
	"encoding/binary"

	"fmt"
	"github.com/ethereum/go-ethereum/common"
)

type Receipt struct {
	HandID    common.Hash    `json:"handId"`
	Index     uint64         `json:"index,string"`
	Kind      string         `json:"kind"`
	Payload   []byte         `json:"payload"`
	Previous  common.Hash    `json:"previous"`
	At        uint64         `json:"at,string"`
	Server    common.Address `json:"server"`
	Digest    common.Hash    `json:"digest"`
	Signature string         `json:"signature"`
}

func (r Receipt) Hash() common.Hash {
	b := append([]byte("ZKPOKER_RECEIPT_V3"), r.HandID[:]...)
	b = binary.BigEndian.AppendUint64(b, r.Index)
	b = binary.BigEndian.AppendUint64(b, r.At)
	b = append(b, r.Server[:]...)
	k := SHA([]byte(r.Kind))
	p := SHA(r.Payload)
	return SHA(b, r.Previous[:], k[:], p[:])
}
func (r Receipt) Verify() error {
	if r.Hash() != r.Digest {
		return fmt.Errorf("receipt digest mismatch")
	}
	return Verify(r.Digest, r.Signature, r.Server)
}
