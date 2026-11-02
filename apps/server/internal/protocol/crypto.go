package protocol

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polygon-zk-poker/server/internal/engine"
)

const MaxActions = 64

var Kinds = map[string]uint8{"fold": 0, "check": 1, "call": 2, "bet": 3, "raise": 4, "all-in": 5}

type Domain struct {
	ChainID  uint64         `json:"chainId"`
	Registry common.Address `json:"registry"`
}
type Authorization struct {
	AccountID  common.Hash    `json:"accountId"`
	PlayerID   common.Hash    `json:"playerId"`
	Issuer     common.Address `json:"issuer"`
	Kind       uint8          `json:"kind"`
	SessionKey common.Address `json:"sessionKey"`
	TableID    common.Hash    `json:"tableId"`
	Nonce      uint64         `json:"nonce,string"`
	ExpiresAt  uint64         `json:"expiresAt,string"`
}
type Session struct {
	Domain          Domain        `json:"domain"`
	Authorization   Authorization `json:"authorization"`
	Signature       string        `json:"signature"`
	IssuerSignature string        `json:"issuerSignature"`
	ID              common.Hash   `json:"id"`
	Revoked         bool          `json:"revoked"`
}
type Spec struct {
	Domain      Domain         `json:"domain"`
	TableID     common.Hash    `json:"tableId"`
	HandID      common.Hash    `json:"handId"`
	Config      engine.Config  `json:"config"`
	AuthIssuer  common.Address `json:"authIssuer"`
	Players     [9]common.Hash `json:"players"`
	Sessions    [9]common.Hash `json:"sessions"`
	ValidUntil  uint64         `json:"validUntil,string"`
	ActionLimit uint16         `json:"actionLimit"`
}
type SignedValue struct {
	Value     common.Hash `json:"value"`
	Signature string      `json:"signature"`
}
type Move struct {
	Action     engine.Action `json:"action"`
	Sequence   uint64        `json:"sequence,string"`
	Previous   common.Hash   `json:"previousState"`
	Signature  string        `json:"signature"`
	State      common.Hash   `json:"state"`
	PublicView common.Hash   `json:"publicView"`
	HoleCommit common.Hash   `json:"holeCommit"`
}

func SHA(parts ...[]byte) common.Hash {
	h := sha256.New()
	for _, b := range parts {
		h.Write(b)
	}
	return common.BytesToHash(h.Sum(nil))
}
func (d Domain) Validate() error {
	if (d.ChainID != 80002 && d.ChainID != 137 && d.ChainID != 31337) || d.Registry == (common.Address{}) {
		return fmt.Errorf("invalid protocol domain")
	}
	return nil
}
func (s Session) Digest() common.Hash {
	a := s.Authorization
	b := []byte("ZKPOKER_SESSION_V3")
	b = binary.BigEndian.AppendUint64(b, s.Domain.ChainID)
	b = append(b, s.Domain.Registry[:]...)
	b = append(b, a.Issuer[:]...)
	b = append(b, a.PlayerID[:]...)
	b = append(b, a.AccountID[:]...)
	b = append(b, a.SessionKey[:]...)
	b = append(b, a.TableID[:]...)
	b = binary.BigEndian.AppendUint64(b, a.Nonce)
	b = binary.BigEndian.AppendUint64(b, a.ExpiresAt)
	b = append(b, a.Kind)
	return SHA(b)
}
func SignatureBytes(s string) ([]byte, error) {
	if len(s) != 132 || !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("signature must be 65-byte hex")
	}
	b, e := hex.DecodeString(s[2:])
	if e != nil {
		return nil, e
	}
	if b[64] >= 27 {
		b[64] -= 27
	}
	if b[64] > 1 || !crypto.ValidateSignatureValues(b[64], new(big.Int).SetBytes(b[:32]), new(big.Int).SetBytes(b[32:64]), true) {
		return nil, fmt.Errorf("noncanonical signature")
	}
	return b, nil
}
func Recover(digest common.Hash, signature string) (common.Address, []byte, error) {
	b, e := SignatureBytes(signature)
	if e != nil {
		return common.Address{}, nil, e
	}
	pub, e := crypto.SigToPub(digest[:], b)
	if e != nil {
		return common.Address{}, nil, e
	}
	return crypto.PubkeyToAddress(*pub), crypto.FromECDSAPub(pub)[1:], nil
}
func Verify(digest common.Hash, signature string, signer common.Address) error {
	got, _, e := Recover(digest, signature)
	if e != nil {
		return e
	}
	if got != signer || signer == (common.Address{}) {
		return fmt.Errorf("signature signer mismatch")
	}
	return nil
}
func Sign(key *ecdsa.PrivateKey, digest common.Hash) (string, error) {
	b, e := crypto.Sign(digest[:], key)
	if e != nil {
		return "", e
	}
	b[64] += 27
	return "0x" + hex.EncodeToString(b), nil
}
func (s *Session) Validate(now uint64) error {
	if e := s.Domain.Validate(); e != nil {
		return e
	}
	a := s.Authorization
	if a.AccountID == (common.Hash{}) || a.PlayerID == (common.Hash{}) || a.Issuer == (common.Address{}) || a.Kind > 1 || a.SessionKey == (common.Address{}) || a.TableID == (common.Hash{}) || a.ExpiresAt <= now || a.ExpiresAt > now+7*86400 {
		return fmt.Errorf("invalid or expired session (maximum 7 days)")
	}
	id := s.Digest()
	if s.ID != (common.Hash{}) && s.ID != id {
		return fmt.Errorf("session ID mismatch")
	}
	if e := s.ValidateOwnership(); e != nil {
		return e
	}
	if e := Verify(id, s.IssuerSignature, a.Issuer); e != nil {
		return fmt.Errorf("invalid identity certificate: %w", e)
	}
	s.ID = id
	s.Revoked = false
	return nil
}
func RevokeDigest(d Domain, id common.Hash) common.Hash {
	b := binary.BigEndian.AppendUint64([]byte("ZKPOKER_REVOKE_V3"), d.ChainID)
	return SHA(b, d.Registry[:], id[:])
}
func (s Spec) Context() common.Hash {
	b := []byte("ZKPOKER_CONTEXT_V3")
	b = binary.BigEndian.AppendUint64(b, s.Domain.ChainID)
	b = append(b, s.Domain.Registry[:]...)
	b = append(b, s.AuthIssuer[:]...)
	b = append(b, s.TableID[:]...)
	b = append(b, s.HandID[:]...)
	for _, v := range s.Config.Stacks {
		b = binary.BigEndian.AppendUint64(b, uint64(v))
	}
	b = append(b, byte(s.Config.Button))
	for _, v := range []int64{s.Config.SmallBlind, s.Config.BigBlind, s.Config.Ante} {
		b = binary.BigEndian.AppendUint64(b, uint64(v))
	}
	for _, p := range s.Players {
		b = append(b, p[:]...)
	}
	for _, id := range s.Sessions {
		b = append(b, id[:]...)
	}
	b = binary.BigEndian.AppendUint64(b, s.ValidUntil)
	b = binary.BigEndian.AppendUint16(b, s.ActionLimit)
	return SHA(b)
}
func SeedCommit(context common.Hash, seat byte, seed common.Hash) common.Hash {
	return SHA([]byte("ZKPOKER_SEED_V3"), context[:], []byte{seat}, seed[:])
}
func Manifest(context, server common.Hash, players [9]SignedValue) common.Hash {
	b := append([]byte("ZKPOKER_MANIFEST_V3"), context[:]...)
	b = append(b, server[:]...)
	for _, p := range players {
		b = append(b, p.Value[:]...)
	}
	return SHA(b)
}
func ValueDigest(kind string, session, hand, context, value common.Hash) common.Hash {
	return SHA([]byte("ZKPOKER_"+kind+"_V3"), session[:], hand[:], context[:], value[:])
}
func ConsentDigest(session, hand, manifest common.Hash) common.Hash {
	return SHA([]byte("ZKPOKER_CONSENT_V3"), session[:], hand[:], manifest[:])
}
func InitialState(context, manifest, deck common.Hash) common.Hash {
	return SHA([]byte("ZKPOKER_INITIAL_V3"), context[:], manifest[:], deck[:])
}
func ActionDigest(session, hand, previous common.Hash, sequence uint64, action engine.Action, observations ...common.Hash) (common.Hash, error) {
	if len(observations) != 2 {
		return common.Hash{}, fmt.Errorf("public and private view commitments required")
	}
	kind, ok := Kinds[action.Kind]
	if !ok || action.Seat < 0 || action.Seat >= 9 || action.Amount < 0 || sequence == 0 {
		return common.Hash{}, fmt.Errorf("invalid action encoding")
	}
	b := append([]byte("ZKPOKER_ACTION_V3"), session[:]...)
	b = append(b, hand[:]...)
	b = append(b, previous[:]...)
	b = binary.BigEndian.AppendUint64(b, sequence)
	b = append(b, byte(action.Seat), kind)
	b = binary.BigEndian.AppendUint64(b, uint64(action.Amount))
	return SHA(b, observations[0][:], observations[1][:]), nil
}

func PlayerID(pub []byte) common.Hash { return SHA([]byte("ZKPOKER_PLAYER_V3"), pub) }
func (s Session) ValidateOwnership() error {
	address, pub, e := Recover(s.Digest(), s.Signature)
	if e != nil {
		return e
	}
	if address != s.Authorization.SessionKey || PlayerID(pub) != s.Authorization.PlayerID {
		return fmt.Errorf("player public key binding mismatch")
	}
	return nil
}
