CREATE TABLE IF NOT EXISTS protocol_sessions_v3 (
 id bytea PRIMARY KEY CHECK(octet_length(id)=32),
 player_id bytea NOT NULL CHECK(octet_length(player_id)=32),
 account_id bytea NOT NULL CHECK(octet_length(account_id)=32),
 chain_id bigint NOT NULL, registry bytea NOT NULL CHECK(octet_length(registry)=20),
 nonce numeric(20,0) NOT NULL, expires_at bigint NOT NULL,
 revoked boolean NOT NULL DEFAULT false, payload jsonb NOT NULL,
 UNIQUE(account_id,chain_id,registry,nonce)
);
CREATE TABLE IF NOT EXISTS protocol_hands_v3 (
 id bytea PRIMARY KEY CHECK(octet_length(id)=32), version bigint NOT NULL,
 snapshot bytea NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS protocol_events_v3 (
 hand_id bytea NOT NULL REFERENCES protocol_hands_v3(id),
 event_index bigint NOT NULL, payload jsonb NOT NULL,
 PRIMARY KEY(hand_id,event_index)
);
CREATE OR REPLACE FUNCTION protect_protocol_events_v3() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'protocol receipt log is append-only'; END $$;
DROP TRIGGER IF EXISTS immutable_protocol_events_v3 ON protocol_events_v3;
CREATE TRIGGER immutable_protocol_events_v3 BEFORE UPDATE OR DELETE ON protocol_events_v3
FOR EACH ROW EXECUTE FUNCTION protect_protocol_events_v3();
CREATE TABLE IF NOT EXISTS identities_v3(
 id bytea PRIMARY KEY CHECK(octet_length(id)=32),kind smallint NOT NULL CHECK(kind IN(0,1)),
 username text UNIQUE,salt bytea,password_hash bytea,
 CHECK((kind=0 AND username IS NULL AND password_hash IS NULL) OR
       (kind=1 AND username IS NOT NULL AND octet_length(salt)=16 AND octet_length(password_hash)=32))
);
CREATE TABLE IF NOT EXISTS login_tokens_v3(
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
 player_id bytea NOT NULL REFERENCES identities_v3(id), expires_at bigint NOT NULL
);

ALTER TABLE protocol_sessions_v3 ADD COLUMN IF NOT EXISTS account_id bytea CHECK(octet_length(account_id)=32);
CREATE UNIQUE INDEX IF NOT EXISTS account_session_nonce_v3 ON protocol_sessions_v3(account_id,chain_id,registry,nonce);
