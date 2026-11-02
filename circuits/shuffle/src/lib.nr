// Constrained permutation primitives. Does not bind swap indices to entropy.
pub fn assert_permutation(deck: [u8; 52]) {
    for i in 0..52 {
        assert(deck[i] < 52, "card range");
        for j in 0..52 {
            if i < j {
                assert(deck[i] != deck[j], "duplicate card");
            }
        }
    }
}

pub fn apply_swaps(swaps: [u8; 51]) -> [u8; 52] {
    let mut deck = [0; 52];
    for i in 0..52 {
        deck[i] = i as u8;
    }
    for k in 0..51 {
        let i = 51 - k;
        let j = swaps[k] as u32;
        assert(j <= i, "swap range");
        let old = deck[i];
        deck[i] = deck[j];
        deck[j] = old;
    }
    assert_permutation(deck);
    deck
}

#[test]
fn valid_swaps() {
    assert_permutation(apply_swaps([0; 51]));
}
#[test(should_fail_with = "duplicate card")]
fn duplicate() {
    assert_permutation([0; 52]);
}
#[test(should_fail_with = "card range")]
fn invalid_card() {
    let mut d = apply_swaps([0; 51]);
    d[0] = 52;
    assert_permutation(d);
}
#[test(should_fail_with = "swap range")]
fn invalid_swap() {
    let mut s = [0; 51];
    s[50] = 2;
    let _d = apply_swaps(s);
}
#[test(should_fail)]
fn wrong_order() {
    let d = apply_swaps([0; 51]);
    let mut alleged = d;
    alleged[0] = d[1];
    alleged[1] = d[0];
    assert(alleged == d);
}
