package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/polygon-zk-poker/server/internal/protocol"
	"strconv"
)

//go:embed migration.sql
var migration string

type Postgres struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Postgres, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	if _, e = p.Exec(ctx, migration); e != nil {
		p.Close()
		return nil, e
	}
	return &Postgres{p}, nil
}
func dbError(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && (p.Code == "23505" || p.Code == "40001") {
		return ErrConflict
	}
	return e
}
func (p *Postgres) PutSession(ctx context.Context, s protocol.Session) error {
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	_, e = p.Pool.Exec(ctx, `INSERT INTO protocol_sessions_v3(id,player_id,account_id,chain_id,registry,nonce,expires_at,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO NOTHING`, s.ID[:], s.Authorization.PlayerID[:], s.Authorization.AccountID[:], s.Domain.ChainID, s.Domain.Registry[:], strconv.FormatUint(s.Authorization.Nonce, 10), s.Authorization.ExpiresAt, b)
	return dbError(e)
}
func (p *Postgres) Session(ctx context.Context, id common.Hash) (protocol.Session, error) {
	var s protocol.Session
	var b []byte
	var revoked bool
	e := p.Pool.QueryRow(ctx, `SELECT payload,revoked FROM protocol_sessions_v3 WHERE id=$1`, id[:]).Scan(&b, &revoked)
	if e != nil {
		return s, dbError(e)
	}
	e = json.Unmarshal(b, &s)
	s.Revoked = revoked
	return s, e
}
func (p *Postgres) Revoke(ctx context.Context, id common.Hash) error {
	tag, e := p.Pool.Exec(ctx, `UPDATE protocol_sessions_v3 SET revoked=true WHERE id=$1`, id[:])
	if e != nil {
		return dbError(e)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func lockSessions(ctx context.Context, tx pgx.Tx, ids []common.Hash, now uint64) error {
	for _, id := range ids {
		var revoked bool
		var expiry uint64
		if e := tx.QueryRow(ctx, `SELECT revoked,expires_at FROM protocol_sessions_v3 WHERE id=$1 FOR SHARE`, id[:]).Scan(&revoked, &expiry); e != nil {
			return dbError(e)
		}
		if revoked || expiry <= now {
			return fmt.Errorf("inactive session")
		}
	}
	return nil
}
func (p *Postgres) Create(ctx context.Context, id common.Hash, snapshot []byte, event protocol.Receipt, ids []common.Hash, now uint64) error {
	tx, e := p.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockSessions(ctx, tx, ids, now); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO protocol_hands_v3(id,version,snapshot) VALUES($1,0,$2)`, id[:], snapshot); e != nil {
		return dbError(e)
	}
	b, _ := json.Marshal(event)
	if _, e = tx.Exec(ctx, `INSERT INTO protocol_events_v3(hand_id,event_index,payload) VALUES($1,0,$2)`, id[:], b); e != nil {
		return dbError(e)
	}
	return dbError(tx.Commit(ctx))
}
func (p *Postgres) Load(ctx context.Context, id common.Hash) (Hand, error) {
	var h Hand
	e := p.Pool.QueryRow(ctx, `SELECT version,snapshot FROM protocol_hands_v3 WHERE id=$1`, id[:]).Scan(&h.Version, &h.Snapshot)
	return h, dbError(e)
}
func (p *Postgres) Commit(ctx context.Context, id common.Hash, version uint64, snapshot []byte, event protocol.Receipt, ids []common.Hash, now uint64) error {
	if event.Index != version+1 {
		return ErrConflict
	}
	tx, e := p.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Lock the hand first and check its CAS version. Session FOR SHARE locks make
	// revocation and action acceptance serialize in the same DB transaction.
	var current uint64
	if e = tx.QueryRow(ctx, `SELECT version FROM protocol_hands_v3 WHERE id=$1 FOR UPDATE`, id[:]).Scan(&current); e != nil {
		return dbError(e)
	}
	if current != version {
		return ErrConflict
	}
	if e = lockSessions(ctx, tx, ids, now); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE protocol_hands_v3 SET version=$2,snapshot=$3,updated_at=now() WHERE id=$1`, id[:], version+1, snapshot); e != nil {
		return dbError(e)
	}
	b, _ := json.Marshal(event)
	if _, e = tx.Exec(ctx, `INSERT INTO protocol_events_v3(hand_id,event_index,payload) VALUES($1,$2,$3)`, id[:], event.Index, b); e != nil {
		return dbError(e)
	}
	return dbError(tx.Commit(ctx))
}
func (p *Postgres) Events(ctx context.Context, id common.Hash, after uint64) ([]protocol.Receipt, error) {
	if _, e := p.Load(ctx, id); e != nil {
		return nil, e
	}
	rows, e := p.Pool.Query(ctx, `SELECT payload FROM protocol_events_v3 WHERE hand_id=$1 AND event_index>=$2 ORDER BY event_index`, id[:], after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []protocol.Receipt{}
	for rows.Next() {
		var b []byte
		var r protocol.Receipt
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
