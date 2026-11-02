// Directly constrained SHA-256 padding and byte/word packing. The only black
// box is the backend's SHA256 compression primitive; no unconstrained helpers.
pub fn digest<let N: u32>(input: [u8; N]) -> [u8; 32] {
    hash_var(input, N)
}
pub fn hash_var<let N: u32>(input: [u8; N], length: u32) -> [u8; 32] {
    assert(length <= N, "SHA256 message length");
    let mut padded: [u8; ((N + 9 + 63) / 64) * 64] = [0; ((N + 9 + 63) / 64) * 64];
    for i in 0..N {
        if i < length {
            padded[i] = input[i];
        }
    }
    padded[length] = 128;
    let blocks = (length + 9 + 63) / 64;
    let bits = (length as u64) * 8;
    for j in 0..8 {
        padded[blocks * 64 - 8 + j] = (bits >> ((8 * (7 - j)) as u64)) as u8;
    }
    let mut state: [u32; 8] = [
        0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
        0x5be0cd19,
    ];
    for block in 0..((N + 9 + 63) / 64) {
        let mut words: [u32; 16] = [0; 16];
        for j in 0..16 {
            let mut value: u32 = 0;
            for k in 0..4 {
                value = value * 256 + padded[block * 64 + j * 4 + k] as u32;
            }
            words[j] = value;
        }
        let compressed = std::hash::sha256_compression(words, state);
        if block < blocks {
            state = compressed;
        }
    }
    let mut result = [0; 32];
    for j in 0..8 {
        for k in 0..4 {
            result[j * 4 + k] = (state[j] >> ((8 * (3 - k)) as u32)) as u8;
        }
    }
    result
}
#[test]
fn known_vectors() {
    assert(
        digest("abc".as_bytes())
            == [
                186, 120, 22, 191, 143, 1, 207, 234, 65, 65, 64, 222, 93, 174, 34, 35, 176, 3, 97,
                163, 150, 23, 122, 156, 180, 16, 255, 97, 242, 0, 21, 173,
            ],
    );
    let mut padded = [0; 80];
    padded[0] = 97;
    padded[1] = 98;
    padded[2] = 99;
    assert(hash_var(padded, 3) == digest("abc".as_bytes()));
}
#[test(should_fail)]
fn changed_input() {
    assert(digest("abd".as_bytes()) == digest("abc".as_bytes()));
}
