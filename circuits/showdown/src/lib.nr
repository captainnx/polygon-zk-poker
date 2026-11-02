fn encode(category: u8, values: [u8; 5]) -> u32 {
    let mut r = category as u32;
    for i in 0..5 {
        r = r * 15 + values[i] as u32;
    }
    r
}
pub fn evaluate5(cards: [u8; 5]) -> u32 {
    let mut counts: [u8; 15] = [0; 15];
    let mut ranks = [0; 5];
    let mut flush = true;
    for i in 0..5 {
        assert(cards[i] < 52, "card range");
        for j in 0..5 {
            if i < j {
                assert(cards[i] != cards[j], "duplicate card");
            }
        }
        let rank = cards[i] / 4 + 2;
        ranks[i] = rank;
        counts[rank as u32] = counts[rank as u32] + 1;
        flush = flush & (cards[i] % 4 == cards[0] % 4);
    }
    let sorted = ranks.sort();
    let mut distinct = true;
    for i in 0..4 {
        distinct = distinct & (sorted[i] != sorted[i + 1]);
    }
    let mut straight: u8 = 0;
    if distinct & (sorted[4] - sorted[0] == 4) {
        straight = sorted[4];
    }
    if sorted == [2, 3, 4, 5, 14] {
        straight = 5;
    }
    let high = [sorted[4], sorted[3], sorted[2], sorted[1], sorted[0]];
    let mut keys: [u8; 13] = [0; 13];
    for i in 0..13 {
        let r = i + 2;
        keys[i] = counts[r] * 16 + r as u8;
    }
    let groups = keys.sort();
    let g0 = groups[12] % 16;
    let g1 = groups[11] % 16;
    let g2 = groups[10] % 16;
    let g3 = groups[9] % 16;
    let c0 = groups[12] / 16;
    let c1 = groups[11] / 16;
    let mut result = encode(0, high);
    if c0 == 2 {
        result = encode(1, [g0, g1, g2, g3, 0]);
    }
    if (c0 == 2) & (c1 == 2) {
        result = encode(2, [g0, g1, g2, 0, 0]);
    }
    if c0 == 3 {
        result = encode(3, [g0, g1, g2, 0, 0]);
    }
    if straight > 0 {
        result = encode(4, [straight, 0, 0, 0, 0]);
    }
    if flush {
        result = encode(5, high);
    }
    if (c0 == 3) & (c1 == 2) {
        result = encode(6, [g0, g1, 0, 0, 0]);
    }
    if c0 == 4 {
        result = encode(7, [g0, g1, 0, 0, 0]);
    }
    if flush & (straight > 0) {
        result = encode(8, [straight, 0, 0, 0, 0]);
    }
    result
}
pub fn evaluate7(cards: [u8; 7]) -> u32 {
    for i in 0..7 {
        assert(cards[i] < 52, "card range");
        for j in 0..7 {
            if i < j {
                assert(cards[i] != cards[j], "duplicate card");
            }
        }
    }
    let combinations: [[u32; 5]; 21] = [
        [0, 1, 2, 3, 4],
        [0, 1, 2, 3, 5],
        [0, 1, 2, 3, 6],
        [0, 1, 2, 4, 5],
        [0, 1, 2, 4, 6],
        [0, 1, 2, 5, 6],
        [0, 1, 3, 4, 5],
        [0, 1, 3, 4, 6],
        [0, 1, 3, 5, 6],
        [0, 1, 4, 5, 6],
        [0, 2, 3, 4, 5],
        [0, 2, 3, 4, 6],
        [0, 2, 3, 5, 6],
        [0, 2, 4, 5, 6],
        [0, 3, 4, 5, 6],
        [1, 2, 3, 4, 5],
        [1, 2, 3, 4, 6],
        [1, 2, 3, 5, 6],
        [1, 2, 4, 5, 6],
        [1, 3, 4, 5, 6],
        [2, 3, 4, 5, 6],
    ];
    let mut best = 0;
    for i in 0..21 {
        let c = combinations[i];
        let rank = evaluate5(
            [cards[c[0]], cards[c[1]], cards[c[2]], cards[c[3]], cards[c[4]]],
        );
        if rank > best {
            best = rank;
        }
    }
    best
}
#[test]
fn categories() {
    assert(evaluate7([32, 36, 40, 44, 48, 1, 6]) / 759375 == 8);
    assert(evaluate7([48, 49, 50, 51, 44, 1, 6]) / 759375 == 7);
    assert(evaluate7([48, 49, 50, 44, 45, 46, 1]) / 759375 == 6);
    assert(evaluate7([48, 40, 28, 16, 4, 1, 6]) / 759375 == 5);
    assert(evaluate7([48, 1, 6, 11, 12, 22, 31]) / 759375 == 4);
    assert(evaluate7([48, 49, 50, 40, 33, 22, 11]) / 759375 == 3);
    assert(evaluate7([48, 49, 44, 45, 36, 21, 10]) / 759375 == 2);
    assert(evaluate7([48, 49, 40, 33, 26, 17, 6]) / 759375 == 1);
    assert(evaluate7([48, 41, 34, 23, 16, 9, 2]) / 759375 == 0);
}
#[test]
fn kickers_wheel_and_tie() {
    assert(evaluate5([48, 49, 50, 51, 44]) > evaluate5([48, 49, 50, 51, 40]));
    assert(evaluate5([0, 5, 10, 15, 16]) > evaluate5([48, 1, 6, 11, 12]));
    assert(evaluate7([0, 5, 32, 36, 40, 44, 48]) == evaluate7([1, 6, 32, 36, 40, 44, 48]));
}
#[test(should_fail_with = "duplicate card")]
fn duplicates() {
    let _r = evaluate7([0, 0, 1, 2, 3, 4, 5]);
}
#[test(should_fail_with = "card range")]
fn range() {
    let _r = evaluate7([52, 0, 1, 2, 3, 4, 5]);
}
#[test(should_fail)]
fn wrong_winner() {
    assert(evaluate7([32, 36, 40, 44, 48, 1, 6]) < evaluate7([48, 49, 50, 51, 44, 1, 6]));
}
