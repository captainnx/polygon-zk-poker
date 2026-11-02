-- Schema foundation only: the demo server does not yet write these tables.
BEGIN;
CREATE TABLE poker_tables (
  table_id bytea PRIMARY KEY CHECK (octet_length(table_id)=32),
  chain_id bigint NOT NULL CHECK (chain_id IN (80002,137,31337)),
  registry bytea NOT NULL CHECK (octet_length(registry)=20),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE session_authorizations (
  session_id bytea PRIMARY KEY CHECK (octet_length(session_id)=32),
  table_id bytea NOT NULL REFERENCES poker_tables(table_id),
  wallet bytea NOT NULL CHECK (octet_length(wallet)=20),
  session_key bytea NOT NULL CHECK (octet_length(session_key)=20),
  chain_id bigint NOT NULL,
  registry bytea NOT NULL CHECK (octet_length(registry)=20),
  nonce numeric(20,0) NOT NULL CHECK (nonce>=0 AND nonce<=18446744073709551615),
  expires_at timestamptz NOT NULL,
  signature bytea NOT NULL CHECK (octet_length(signature)=65),
  revoked_at timestamptz,
  UNIQUE(wallet, chain_id, registry, nonce)
);
CREATE TABLE hands (
  hand_id bytea PRIMARY KEY CHECK (octet_length(hand_id)=32),
  table_id bytea NOT NULL REFERENCES poker_tables(table_id),
  status text NOT NULL CHECK (status IN ('committed','revealing','playing','finished','proved','submitted','aborted')),
  config jsonb NOT NULL,
  roster jsonb NOT NULL,
  server_commit bytea NOT NULL CHECK (octet_length(server_commit)=32),
  protocol_commitment bytea CHECK (octet_length(protocol_commitment)=32),
  final_stacks jsonb,
  abort_reason text,
  result_tx bytea CHECK (octet_length(result_tx)=32),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE signed_actions (
  hand_id bytea NOT NULL REFERENCES hands(hand_id),
  sequence bigint NOT NULL CHECK (sequence>0),
  session_id bytea NOT NULL REFERENCES session_authorizations(session_id),
  seat smallint NOT NULL CHECK (seat BETWEEN 0 AND 8),
  previous_state bytea NOT NULL CHECK (octet_length(previous_state)=32),
  action jsonb NOT NULL,
  signature bytea NOT NULL CHECK (octet_length(signature)=65),
  resulting_state bytea NOT NULL CHECK (octet_length(resulting_state)=32),
  received_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(hand_id, sequence)
);
CREATE TABLE protocol_receipts (
  hand_id bytea NOT NULL REFERENCES hands(hand_id),
  receipt_index bigint NOT NULL CHECK (receipt_index>=0),
  kind text NOT NULL,
  payload bytea NOT NULL,
  signature bytea NOT NULL CHECK (octet_length(signature)=65),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(hand_id, receipt_index)
);
CREATE FUNCTION reject_log_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'protocol logs are append-only'; END $$;
CREATE TRIGGER immutable_actions BEFORE UPDATE OR DELETE ON signed_actions
FOR EACH ROW EXECUTE FUNCTION reject_log_mutation();
CREATE TRIGGER immutable_receipts BEFORE UPDATE OR DELETE ON protocol_receipts
FOR EACH ROW EXECUTE FUNCTION reject_log_mutation();
COMMIT;
