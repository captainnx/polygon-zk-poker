package identity

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ Pool *pgxpool.Pool }

func (p *Postgres) Guest(ctx context.Context, v Principal) error {
	_, e := p.Pool.Exec(ctx, `INSERT INTO identities_v3(id,kind) VALUES($1,0)`, v.ID[:])
	return e
}
func (p *Postgres) Register(ctx context.Context, a Credential) error {
	tag, e := p.Pool.Exec(ctx, `INSERT INTO identities_v3(id,kind,username,salt,password_hash) VALUES($1,1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET kind=1,username=$2,salt=$3,password_hash=$4 WHERE identities_v3.kind=0`, a.Principal.ID[:], a.Principal.Username, a.Salt, a.Hash)
	var pe *pgconn.PgError
	if errors.As(e, &pe) && pe.Code == "23505" {
		return ErrConflict
	}
	if e == nil && tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return e
}
func (p *Postgres) Account(ctx context.Context, u string) (Credential, error) {
	var a Credential
	var id []byte
	e := p.Pool.QueryRow(ctx, `SELECT id,kind,username,salt,password_hash FROM identities_v3 WHERE username=$1 AND kind=1`, u).Scan(&id, &a.Principal.Kind, &a.Principal.Username, &a.Salt, &a.Hash)
	a.Principal.ID = common.BytesToHash(id)
	return a, e
}
func (p *Postgres) Token(ctx context.Context, t, id common.Hash, expiry uint64) error {
	_, e := p.Pool.Exec(ctx, `INSERT INTO login_tokens_v3(token_hash,player_id,expires_at) VALUES($1,$2,$3)`, t[:], id[:], expiry)
	return e
}
func (p *Postgres) Resolve(ctx context.Context, t common.Hash, now uint64) (Principal, error) {
	var v Principal
	var id []byte
	e := p.Pool.QueryRow(ctx, `SELECT i.id,i.kind,coalesce(i.username,'') FROM login_tokens_v3 t JOIN identities_v3 i ON i.id=t.player_id WHERE t.token_hash=$1 AND t.expires_at>$2`, t[:], now).Scan(&id, &v.Kind, &v.Username)
	if e != nil {
		return v, ErrUnauthenticated
	}
	v.ID = common.BytesToHash(id)
	return v, nil
}
func (p *Postgres) Logout(ctx context.Context, t common.Hash) error {
	_, e := p.Pool.Exec(ctx, `DELETE FROM login_tokens_v3 WHERE token_hash=$1`, t[:])
	return e
}
