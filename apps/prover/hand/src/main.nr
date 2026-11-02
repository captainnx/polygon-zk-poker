use dep::poker_crypto as c;
use dep::poker_engine as engine;

struct Authorization {
 account_id:[u8;32],
    session: c::PublicKey,
    signature: [u8; 64],
    issuer_signature:[u8;64],
    kind:u8,
    nonce: u64,
    expiry: u64,
}
struct Move {
    seat: u32,
    kind: u8,
    amount: u64,
    signature: [u8; 64],
}

// 44 public fields bind chain, registry, hand limbs, nine player ID limbs,
// initial/final chips, result limbs, deadline and the pinned identity issuer.
fn main(
    statement: pub [Field; 44],
    issuer_key:c::PublicKey,
    table: [u8; 32],
    button: u32,
    small: u64,
    big: u64,
    ante: u64,
    action_limit: u16,
    authorizations: [Authorization; 9],
    server_seed: [u8; 32],
    player_seeds: [[u8; 32]; 9],
    consent_signatures: [[u8; 64]; 9],
    deck: [u8; 52],
    action_count: u32,
    moves: [Move; 64],
) {
    let chain = c::as_u64(statement[0]);
    assert((chain == 31337) | (chain == 80002) | (chain == 137), "unsupported chain");
    let registry = c::address(statement[1]);
    assert(registry != [0; 20]);
    let hand = c::bytes32(statement[2], statement[3]);
    assert(hand != [0; 32]);
    assert(table != [0; 32]);
    let valid_until = c::as_u64(statement[42]);
 let issuer=c::address(statement[43]);assert(issuer!=[0;20]);assert(c::hash_address(issuer_key)==issuer,"identity issuer binding");
    assert(valid_until > 0);
    assert(
        (action_limit > 0) & (action_limit <= 64) & (action_count <= action_limit as u32),
        "action capacity",
    );
    let mut occupied = [false; 9];
    let mut players = [[0; 32]; 9];
    let mut session_ids = [[0; 32]; 9];
    let mut initial = [0; 9];
    let mut expected_final = [0; 9];
    for i in 0..9 {
        players[i]=c::bytes32(statement[4+2*i],statement[5+2*i]);
        initial[i] = c::as_u64(statement[22 + i]);
        expected_final[i] = c::as_u64(statement[31 + i]);
        occupied[i] = initial[i] > 0;
        if occupied[i] {
            assert(players[i] != [0; 32]);
            for j in 0..9 {
                if j < i {
                    assert(players[i] != players[j], "duplicate identity");
                }
            }
            let a = authorizations[i];
            assert(a.kind<=1,"identity kind");
 assert(a.account_id!=[0;32],"account identity");
 assert(c::sha("ZKPOKER_PLAYER_V3".as_bytes().concat(a.session.x).concat(a.session.y))==players[i],"player public key binding");
 for j in 0..9{if j<i {assert(a.account_id!=authorizations[j].account_id,"duplicate account");}}
            let key = c::hash_address(a.session);
            assert(key != [0; 20]);
            assert(a.expiry >= valid_until, "session expired");
            session_ids[i] = c::certificate(chain,registry,issuer,players[i],a.account_id,key,table,a.nonce,a.expiry,a.kind);
 c::verify(issuer_key,a.issuer_signature,session_ids[i]);
 c::verify(a.session, a.signature, session_ids[i]);
        } else {
            let a = authorizations[i];
            assert(
                (a.account_id==[0;32]) & (a.session.x == [0; 32])
                    & (a.session.y == [0; 32])
                    & (a.signature == [0; 64])
                    & (a.issuer_signature==[0;64]) & (a.kind==0)
 & (a.nonce == 0)
                    & (a.expiry == 0),
                "authorization padding",
            );
            assert(
                (players[i] == [0; 32])
                    & (expected_final[i] == 0)
                    & (player_seeds[i] == [0; 32])
                    & (consent_signatures[i] == [0; 64]),
                "vacant seat padding",
            );
        }
    }
    let mut initial_bytes = [0; 72];
    let mut player_bytes = [0; 288];
    let mut session_bytes = [0; 288];
    for i in 0..9 {
        let value = c::be64(initial[i]);
        for j in 0..8 {
            initial_bytes[i * 8 + j] = value[j];
        }
        for j in 0..32 {
            player_bytes[i * 32 + j] = players[i][j];
        }
        for j in 0..32 {
            session_bytes[i * 32 + j] = session_ids[i][j];
        }
    }
    assert(button < 9);
    let context = c::sha("ZKPOKER_CONTEXT_V3"
        .as_bytes()
        .concat(c::be64(chain))
        .concat(registry)
 .concat(issuer)
        .concat(table)
        .concat(hand)
        .concat(initial_bytes)
        .concat([button as u8])
        .concat(c::be64(small))
        .concat(c::be64(big))
        .concat(c::be64(ante))
        .concat(player_bytes)
        .concat(session_bytes)
        .concat(c::be64(valid_until))
        .concat([(action_limit >> 8) as u8, action_limit as u8]));
    let server_commit = c::seed_commit(context, 255, server_seed);
    let mut commits = [[0; 32]; 9];
    for i in 0..9 {
        if occupied[i] {
            commits[i] = c::seed_commit(context, i as u8, player_seeds[i]);
        }
    }
    let manifest = c::manifest(context, server_commit, commits);
    for i in 0..9 {
        if occupied[i] {
            c::verify(
                authorizations[i].session,
                consent_signatures[i],
                c::consent(session_ids[i], hand, manifest),
            );
        }
    }
    let deck_commit = c::shuffled(context, server_seed, player_seeds, occupied, deck);
    let config = engine::Config { initial, button, small, big, ante };
    let mut state = engine::initialize(config, deck);
    let mut holes = [[0; 32]; 9];
    for i in 0..9 {
        if occupied[i] {
            holes[i] = c::hole(context, i as u8, player_seeds[i], state.holes[i]);
        }
    }
    let mut chain_hash = c::initial(context, manifest, deck_commit);
    for i in 0..64 {
        let m = moves[i];
        if i < action_count {
            assert(m.seat < 9);
            assert(occupied[m.seat], "vacant action seat");
            let digest = c::action(
                session_ids[m.seat],
                hand,
                chain_hash,
                (i + 1) as u64,
                m.seat as u8,
                m.kind,
                m.amount,
                engine::view(state, i),
                holes[m.seat],
            );
            c::verify(authorizations[m.seat].session, m.signature, digest);
            state = engine::apply(
                state,
                config,
                deck,
                engine::Action { seat: m.seat, kind: m.kind, amount: m.amount },
            );
            chain_hash = digest;
        } else {
            assert(
                (m.seat == 0) & (m.kind == 0) & (m.amount == 0) & (m.signature == [0; 64]),
                "action padding",
            );
        }
    }
    assert(engine::settle(state, config) == expected_final, "incorrect final stacks");
    assert(
        c::result(manifest, deck_commit, chain_hash) == c::bytes32(statement[40], statement[41]),
        "protocol result binding",
    );
}
