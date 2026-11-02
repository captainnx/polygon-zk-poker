pub mod sha;
use dep::keccak256::keccak256;
use dep::shuffle;

pub struct PublicKey {
    pub x: [u8; 32],
    pub y: [u8; 32],
}
pub fn sha<let N: u32>(input: [u8; N]) -> [u8; 32] {
    sha::digest(input)
}
pub fn be64(value: u64) -> [u8; 8] {
    let mut out = [0; 8];
    for i in 0..8 {
        out[i] = (value >> ((8 * (7 - i)) as u64)) as u8;
    }
    out
}
pub fn word(value: u64) -> [u8; 32] {
    let mut out = [0; 32];
    let bytes = be64(value);
    for i in 0..8 {
        out[24 + i] = bytes[i];
    }
    out
}
pub fn address(field: Field) -> [u8; 20] {
    let bytes = field.to_be_bytes::<32>();
    for i in 0..12 {
        assert(bytes[i] == 0, "address range");
    }
    let mut out = [0; 20];
    for i in 0..20 {
        out[i] = bytes[12 + i];
    }
    out
}
pub fn address_word(value: [u8; 20]) -> [u8; 32] {
    let mut out = [0; 32];
    for i in 0..20 {
        out[12 + i] = value[i];
    }
    out
}
pub fn hash_address(key: PublicKey) -> [u8; 20] {
    let h = keccak256(key.x.concat(key.y), 64);
    let mut out = [0; 20];
    for i in 0..20 {
        out[i] = h[12 + i];
    }
    out
}
pub fn bytes32(hi: Field, lo: Field) -> [u8; 32] {
    let a = hi as u128;
    let b = lo as u128;
    assert((a as Field == hi) & (b as Field == lo), "digest limb range");
    {
        let ah = hi.to_be_bytes::<32>();
        let bl = lo.to_be_bytes::<32>();
        let mut out = [0; 32];
        for i in 0..16 {
            out[i] = ah[16 + i];
            out[16 + i] = bl[16 + i];
        }
        out
    }
}
pub fn as_u64(value: Field) -> u64 {
    let v = value as u64;
    assert(v as Field == value, "uint64 range");
    v
}
pub fn verify(key: PublicKey, signature: [u8; 64], digest: [u8; 32]) {
    assert(
        std::ecdsa_secp256k1::verify_signature(key.x, key.y, signature, digest),
        "invalid signature",
    );
}
pub fn certificate(chain:u64,registry:[u8;20],issuer:[u8;20],player:[u8;32],account:[u8;32],key:[u8;20],table:[u8;32],nonce:u64,expiry:u64,kind:u8)->[u8;32]{
 sha("ZKPOKER_SESSION_V3".as_bytes().concat(be64(chain)).concat(registry).concat(issuer).concat(player).concat(account).concat(key).concat(table).concat(be64(nonce)).concat(be64(expiry)).concat([kind]))
}
pub fn seed_commit(context: [u8; 32], seat: u8, seed: [u8; 32]) -> [u8; 32] {
    sha(
        "ZKPOKER_SEED_V3".as_bytes().concat(context).concat([seat]).concat(seed),
    )
}
pub fn manifest(context: [u8; 32], server: [u8; 32], commits: [[u8; 32]; 9]) -> [u8; 32] {
    let mut joined = [0; 288];
    for i in 0..9 {
        for j in 0..32 {
            joined[i * 32 + j] = commits[i][j];
        }
    }
    sha(
        "ZKPOKER_MANIFEST_V3".as_bytes().concat(context).concat(server).concat(joined),
    )
}
pub fn consent(session: [u8; 32], hand: [u8; 32], manifest: [u8; 32]) -> [u8; 32] {
    sha(
        "ZKPOKER_CONSENT_V3".as_bytes().concat(session).concat(hand).concat(manifest),
    )
}
pub fn action(
    session: [u8; 32],
    hand: [u8; 32],
    previous: [u8; 32],
    sequence: u64,
    seat: u8,
    kind: u8,
    amount: u64,
    view: [u8; 32],
    hole: [u8; 32],
) -> [u8; 32] {
    sha("ZKPOKER_ACTION_V3"
        .as_bytes()
        .concat(session)
        .concat(hand)
        .concat(previous)
        .concat(be64(sequence))
        .concat([seat, kind])
        .concat(be64(amount)).concat(view).concat(hole))
}
pub fn initial(context: [u8; 32], manifest: [u8; 32], deck: [u8; 32]) -> [u8; 32] {
    sha(
        "ZKPOKER_INITIAL_V3".as_bytes().concat(context).concat(manifest).concat(deck),
    )
}
pub fn result(manifest: [u8; 32], deck: [u8; 32], state: [u8; 32]) -> [u8; 32] {
    sha(
        "ZKPOKER_RESULT_V3".as_bytes().concat(manifest).concat(deck).concat(state),
    )
}
fn less(a: [u8; 32], b: [u8; 32]) -> bool {
    let mut prefix = true;
    let mut lower = false;
    for i in 0..32 {
        lower = lower | (prefix & (a[i] < b[i]));
        prefix = prefix & (a[i] == b[i]);
    }
    lower
}
pub fn shuffled(
    context: [u8; 32],
    server: [u8; 32],
    seeds: [[u8; 32]; 9],
    occupied: [bool; 9],
    deck: [u8; 52],
) -> [u8; 32] {
    let prefix = "ZKPOKER_MIX_V3".as_bytes().concat(context).concat(server);
    let mut msg = [0; 375];
    for i in 0..prefix.len() {
        msg[i] = prefix[i];
    }
    let mut length = prefix.len();
    for i in 0..9 {
        if occupied[i] {
            msg[length] = i as u8;
            length = length + 1;
            for j in 0..32 {
                msg[length + j] = seeds[i][j];
            }
            length = length + 32;
        }
    }
    let key = sha::hash_var(msg, length);
    shuffle::assert_permutation(deck);
    let mut previous = [0; 32];
    for i in 0..52 {
        let priority = sha("ZKPOKER_CARD_V3".as_bytes().concat(key).concat([deck[i]]));
        if i > 0 {
            assert(
                less(previous, priority) | ((previous == priority) & (deck[i - 1] < deck[i])),
                "deck does not follow committed entropy",
            );
        }
        previous = priority;
    }
    sha("ZKPOKER_DECK_V3".as_bytes().concat(context).concat(deck))
}

pub fn hole(context: [u8; 32], seat: u8, seed: [u8; 32], cards: [u8; 2]) -> [u8; 32] {
 sha("ZKPOKER_HOLE_V3".as_bytes().concat(context).concat([seat]).concat(seed).concat(cards))
}
