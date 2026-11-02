package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"golang.org/x/crypto/argon2"
)

var ErrCredentials = errors.New("invalid username or password")
var ErrConflict = errors.New("username unavailable")
var ErrUnauthenticated = errors.New("login required")

type Principal struct {
	ID       common.Hash `json:"identityId"`
	Kind     uint8       `json:"kind"`
	Username string      `json:"username,omitempty"`
}
type Credential struct {
	Principal Principal
	Salt      []byte
	Hash      []byte
}
type Store interface {
	Guest(context.Context, Principal) error
	Register(context.Context, Credential) error
	Account(context.Context, string) (Credential, error)
	Token(context.Context, common.Hash, common.Hash, uint64) error
	Resolve(context.Context, common.Hash, uint64) (Principal, error)
	Logout(context.Context, common.Hash) error
}
type Login struct {
	Principal Principal
	Token     string
	Expires   uint64
}
type Service struct {
	Store Store
	Now   func() time.Time
	slots chan struct{}
}

func New(store Store) *Service { return &Service{store, time.Now, make(chan struct{}, 2)} }
func random(n int) []byte {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return b
}
func TokenHash(token string) common.Hash {
	return protocol.SHA([]byte("ZKPOKER_LOGIN_V3"), []byte(token))
}
func (s *Service) login(ctx context.Context, p Principal) (Login, error) {
	token := hex.EncodeToString(random(32))
	expiry := uint64(s.Now().Unix()) + 7*86400
	e := s.Store.Token(ctx, TokenHash(token), p.ID, expiry)
	return Login{p, token, expiry}, e
}
func (s *Service) Guest(ctx context.Context) (Login, error) {
	p := Principal{ID: common.BytesToHash(random(32)), Kind: 0}
	if e := s.Store.Guest(ctx, p); e != nil {
		return Login{}, e
	}
	return s.login(ctx, p)
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

func normalize(u string) string { return strings.ToLower(strings.TrimSpace(u)) }
func (s *Service) derive(ctx context.Context, password string, salt []byte) ([]byte, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32), nil
}
func (s *Service) Register(ctx context.Context, user, password string, current *Principal) (Login, error) {
	user = normalize(user)
	if !usernamePattern.MatchString(user) || len(password) < 12 || len(password) > 256 {
		return Login{}, fmt.Errorf("username must be 3–32 letters/digits/underscore; password must be 12–256 bytes")
	}
	p := Principal{ID: common.BytesToHash(random(32)), Kind: 1, Username: user}
	if current != nil && current.Kind == 0 {
		p.ID = current.ID
	}
	salt := random(16)
	h, e := s.derive(ctx, password, salt)
	if e != nil {
		return Login{}, e
	}
	if e = s.Store.Register(ctx, Credential{p, salt, h}); e != nil {
		return Login{}, e
	}
	return s.login(ctx, p)
}
func (s *Service) Login(ctx context.Context, user, password string) (Login, error) {
	if len(password) > 256 {
		return Login{}, ErrCredentials
	}
	a, e := s.Store.Account(ctx, normalize(user))
	missing := e != nil
	if missing {
		a = Credential{Salt: make([]byte, 16), Hash: make([]byte, 32)}
	}
	h, e := s.derive(ctx, password, a.Salt)
	if e != nil {
		return Login{}, e
	}
	if subtle.ConstantTimeCompare(h, a.Hash) != 1 || missing {
		return Login{}, ErrCredentials
	}
	return s.login(ctx, a.Principal)
}
func (s *Service) Resolve(ctx context.Context, token string) (Principal, error) {
	if len(token) != 64 {
		return Principal{}, ErrUnauthenticated
	}
	return s.Store.Resolve(ctx, TokenHash(token), uint64(s.Now().Unix()))
}
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.Logout(ctx, TokenHash(token))
}

type Memory struct {
	mu       sync.Mutex
	people   map[common.Hash]Principal
	accounts map[string]Credential
	tokens   map[common.Hash]struct {
		ID     common.Hash
		Expiry uint64
	}
}

func NewMemory() *Memory {
	return &Memory{people: map[common.Hash]Principal{}, accounts: map[string]Credential{}, tokens: map[common.Hash]struct {
		ID     common.Hash
		Expiry uint64
	}{}}
}
func (m *Memory) Guest(_ context.Context, p Principal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.people[p.ID] = p
	return nil
}
func (m *Memory) Register(_ context.Context, a Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accounts[a.Principal.Username]; ok {
		return ErrConflict
	}
	if p, ok := m.people[a.Principal.ID]; ok && p.Kind != 0 {
		return ErrConflict
	}
	m.people[a.Principal.ID] = a.Principal
	m.accounts[a.Principal.Username] = a
	return nil
}
func (m *Memory) Account(_ context.Context, u string) (Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[u]
	if !ok {
		return a, ErrCredentials
	}
	return a, nil
}
func (m *Memory) Token(_ context.Context, t, id common.Hash, expiry uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[t] = struct {
		ID     common.Hash
		Expiry uint64
	}{id, expiry}
	return nil
}
func (m *Memory) Resolve(_ context.Context, t common.Hash, now uint64) (Principal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tokens[t]
	if !ok || now >= v.Expiry {
		return Principal{}, ErrUnauthenticated
	}
	return m.people[v.ID], nil
}
func (m *Memory) Logout(_ context.Context, t common.Hash) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, t)
	return nil
}
