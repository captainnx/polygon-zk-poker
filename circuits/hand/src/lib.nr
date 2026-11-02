// Settlement library used by poker_engine. The complete signed relation is
// apps/prover/hand/src/main.nr, which derives ranks with showdown::evaluate7.
pub fn assert_conservation(initial: [u64; 9], final_stacks: [u64; 9]) {
    let mut a: u64 = 0;
    let mut b: u64 = 0;
    for i in 0..9 {
        assert(initial[i] <= 1000000000000, "initial range");
        assert(final_stacks[i] <= 9000000000000, "final range");
        a = a + initial[i];
        b = b + final_stacks[i];
    }
    assert(a == b, "chip conservation");
}

// ranks must be computed with showdown::evaluate7 by the calling hand circuit.
// Contributions are after uncalled-bet refunds, matching the Go engine.
pub fn distribute(
    contributions: [u64; 9],
    eligible: [bool; 9],
    ranks: [u32; 9],
    button: u32,
) -> [u64; 9] {
    assert(button < 9, "button range");
    let mut payout: [u64; 9] = [0; 9];
    for i in 0..9 {
        assert(contributions[i] <= 1000000000000, "contribution range");
    }
    for tier in 0..9 {
        let level = contributions[tier];
        let mut unique = level > 0;
        let mut previous: u64 = 0;
        for j in 0..9 {
            if (j < tier) & (contributions[j] == level) {
                unique = false;
            }
            if (contributions[j] < level) & (contributions[j] > previous) {
                previous = contributions[j];
            }
        }
        if unique {
            let mut amount: u64 = 0;
            let mut best: u32 = 0;
            for j in 0..9 {
                if contributions[j] >= level {
                    amount = amount + level - previous;
                    if eligible[j] & (ranks[j] > best) {
                        best = ranks[j];
                    }
                }
            }
            let mut winners: u64 = 0;
            for j in 0..9 {
                if (contributions[j] >= level) & eligible[j] & (ranks[j] == best) {
                    winners = winners + 1;
                }
            }
            assert(winners > 0, "no eligible winner");
            let share = amount / winners;
            let mut remainder = amount % winners;
            for offset in 1..10 {
                let seat = (button + offset) % 9;
                if (contributions[seat] >= level) & eligible[seat] & (ranks[seat] == best) {
                    payout[seat] = payout[seat] + share;
                    if remainder > 0 {
                        payout[seat] = payout[seat] + 1;
                        remainder = remainder - 1;
                    }
                }
            }
        }
    }
    payout
}
#[test]
fn side_pots() {
    assert(
        distribute(
            [100, 200, 300, 300, 0, 0, 0, 0, 0],
            [true, true, true, false, false, false, false, false, false],
            [9, 8, 7, 99, 0, 0, 0, 0, 0],
            3,
        )
            == [400, 300, 200, 0, 0, 0, 0, 0, 0],
    );
    assert(
        distribute(
            [5, 5, 5, 10, 10, 0, 0, 0, 0],
            [true, true, false, true, true, false, false, false, false],
            [9, 9, 99, 4, 4, 0, 0, 0, 0],
            0,
        )
            == [12, 13, 0, 5, 5, 0, 0, 0, 0],
    );
    assert_conservation(
        [100, 200, 300, 300, 0, 0, 0, 0, 0],
        [400, 300, 200, 0, 0, 0, 0, 0, 0],
    );
}
#[test(should_fail_with = "chip conservation")]
fn illegal_output() {
    assert_conservation([100; 9], [101; 9]);
}
#[test(should_fail)]
fn wrong_side_pot() {
    assert(
        distribute(
            [5, 5, 5, 10, 10, 0, 0, 0, 0],
            [true, true, false, true, true, false, false, false, false],
            [9, 9, 99, 4, 4, 0, 0, 0, 0],
            0,
        )
            == [13, 12, 0, 5, 5, 0, 0, 0, 0],
    );
}
#[test(should_fail_with = "no eligible winner")]
fn ineligible() {
    let _p = distribute([10; 9], [false; 9], [1; 9], 0);
}
