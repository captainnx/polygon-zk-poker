package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"sync"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("version or nonce conflict")

type Hand struct {
	Version  uint64
	Snapshot []byte
}
type Store interface {
	PutSession(context.Context, protocol.Session) error
	Session(context.Context, common.Hash) (protocol.Session, error)
	Revoke(context.Context, common.Hash) error
	Create(context.Context, common.Hash, []byte, protocol.Receipt, []common.Hash, uint64) error
	Load(context.Context, common.Hash) (Hand, error)
	Commit(context.Context, common.Hash, uint64, []byte, protocol.Receipt, []common.Hash, uint64) error
	Events(context.Context, common.Hash, uint64) ([]protocol.Receipt, error)
}
type Memory struct {
	mu       sync.Mutex
	sessions map[common.Hash]protocol.Session
	nonces   map[string]common.Hash
	hands    map[common.Hash]Hand
	events   map[common.Hash][]protocol.Receipt
}

func NewMemory() *Memory {
	return &Memory{sessions: map[common.Hash]protocol.Session{}, nonces: map[string]common.Hash{}, hands: map[common.Hash]Hand{}, events: map[common.Hash][]protocol.Receipt{}}
}
func nonceKey(s protocol.Session) string {
	return fmt.Sprintf("%d/%s/%s/%d", s.Domain.ChainID, s.Domain.Registry.Hex(), s.Authorization.AccountID.Hex(), s.Authorization.Nonce)
}
func (m *Memory) PutSession(_ context.Context, s protocol.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.sessions[s.ID]; ok {
		if old.Digest() != s.Digest() {
			return ErrConflict
		}
		return nil
	}
	if _, ok := m.nonces[nonceKey(s)]; ok {
		return ErrConflict
	}
	m.sessions[s.ID] = s
	m.nonces[nonceKey(s)] = s.ID
	return nil
}
func (m *Memory) Session(_ context.Context, id common.Hash) (protocol.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return s, ErrNotFound
	}
	return s, nil
}
func (m *Memory) Revoke(_ context.Context, id common.Hash) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	s.Revoked = true
	m.sessions[id] = s
	return nil
}
func (m *Memory) active(ids []common.Hash, now uint64) error {
	for _, id := range ids {
		s, ok := m.sessions[id]
		if !ok || s.Revoked || s.Authorization.ExpiresAt <= now {
			return fmt.Errorf("inactive session")
		}
	}
	return nil
}
func (m *Memory) Create(_ context.Context, id common.Hash, snapshot []byte, event protocol.Receipt, ids []common.Hash, now uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.hands[id]; ok {
		return ErrConflict
	}
	if e := m.active(ids, now); e != nil {
		return e
	}
	m.hands[id] = Hand{0, append([]byte(nil), snapshot...)}
	m.events[id] = []protocol.Receipt{event}
	return nil
}
func (m *Memory) Load(_ context.Context, id common.Hash) (Hand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hands[id]
	if !ok {
		return h, ErrNotFound
	}
	h.Snapshot = append([]byte(nil), h.Snapshot...)
	return h, nil
}
func (m *Memory) Commit(_ context.Context, id common.Hash, version uint64, snapshot []byte, event protocol.Receipt, ids []common.Hash, now uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hands[id]
	if !ok {
		return ErrNotFound
	}
	if h.Version != version || event.Index != version+1 {
		return ErrConflict
	}
	if e := m.active(ids, now); e != nil {
		return e
	}
	m.hands[id] = Hand{version + 1, append([]byte(nil), snapshot...)}
	m.events[id] = append(m.events[id], event)
	return nil
}
func (m *Memory) Events(_ context.Context, id common.Hash, after uint64) ([]protocol.Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.hands[id]; !ok {
		return nil, ErrNotFound
	}
	v := []protocol.Receipt{}
	for _, r := range m.events[id] {
		if r.Index >= after {
			b, _ := json.Marshal(r)
			var copy protocol.Receipt
			_ = json.Unmarshal(b, &copy)
			v = append(v, copy)
		}
	}
	return v, nil
}
