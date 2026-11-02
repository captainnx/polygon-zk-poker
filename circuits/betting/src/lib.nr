pub struct State {
    pub stack: u64,
    pub street_bet: u64,
    pub current_bet: u64,
    pub min_raise: u64,
    pub acted: bool,
    pub last_bet: u64,
    pub can_respond: bool,
}
pub struct Result {
    pub stack: u64,
    pub street_bet: u64,
    pub current_bet: u64,
    pub min_raise: u64,
    pub folded: bool,
}

// Actor-local constrained transition; turn order and street advancement are not
// provided by this library. Kind codes match packages/protocol.
pub fn transition(s: State, kind: u8, amount: u64) -> Result {
    assert((s.stack > 0) & (s.stack <= 1000000000000), "stack range");
    assert((s.street_bet <= 1000000000000) & (s.current_bet <= 1000000000000));
    assert((s.min_raise > 0) & (s.min_raise <= 1000000000000));
    assert(s.last_bet <= s.current_bet);
    assert(kind <= 5, "action kind");
    assert(amount <= 1000000000000);
    if (kind != 3) & (kind != 4) {
        assert(amount == 0, "unexpected amount");
    }
    let mut r = Result {
        stack: s.stack,
        street_bet: s.street_bet,
        current_bet: s.current_bet,
        min_raise: s.min_raise,
        folded: false,
    };
    let mut k = kind;
    let mut target = amount;
    if k == 5 {
        target = s.stack + s.street_bet;
        k = if target <= s.current_bet {
            2
        } else {
            if s.current_bet == 0 {
                3
            } else {
                4
            }
        };
    }
    if k == 0 {
        r.folded = true;
    }
    if k == 1 {
        assert(s.street_bet >= s.current_bet, "check facing bet");
    }
    if k == 2 {
        assert(s.street_bet < s.current_bet, "nothing to call");
        let owed = s.current_bet - s.street_bet;
        let paid = if owed < s.stack { owed } else { s.stack };
        r.stack = s.stack - paid;
        r.street_bet = s.street_bet + paid;
    }
    if (k == 3) | (k == 4) {
        assert((k == 3) == (s.current_bet == 0), "bet or raise kind");
        assert(s.can_respond, "dry side pot");
        assert(!s.acted | (s.current_bet - s.last_bet >= s.min_raise), "not reopened");
        assert((target > s.current_bet) & (target <= s.stack + s.street_bet), "raise target");
        let increase = target - s.current_bet;
        assert(
            (increase >= s.min_raise) | (target == s.stack + s.street_bet),
            "short raise must be all-in",
        );
        r.stack = s.stack - (target - s.street_bet);
        r.street_bet = target;
        r.current_bet = target;
        if increase >= s.min_raise {
            r.min_raise = increase;
        }
    }
    assert(r.stack + r.street_bet == s.stack + s.street_bet, "local conservation");
    r
}
fn example() -> State {
    State {
        stack: 90,
        street_bet: 10,
        current_bet: 20,
        min_raise: 10,
        acted: false,
        last_bet: 0,
        can_respond: true,
    }
}
#[test]
fn call_and_raise() {
    let r = transition(example(), 2, 0);
    assert((r.stack == 80) & (r.street_bet == 20));
    let r = transition(example(), 4, 40);
    assert((r.stack == 60) & (r.min_raise == 20));
}
#[test]
fn short_and_cumulative() {
    let mut s = example();
    s.stack = 15;
    let r = transition(s, 5, 0);
    assert((r.current_bet == 25) & (r.min_raise == 10) & (r.stack == 0));
    let mut s = example();
    s.acted = true;
    s.last_bet = 10;
    let r = transition(s, 4, 30);
    assert(r.current_bet == 30);
}
#[test(should_fail_with = "check facing bet")]
fn illegal_check() {
    let _r = transition(example(), 1, 0);
}
#[test(should_fail_with = "short raise must be all-in")]
fn illegal_raise() {
    let _r = transition(example(), 4, 21);
}
#[test(should_fail_with = "not reopened")]
fn short_does_not_reopen() {
    let mut s = example();
    s.acted = true;
    s.last_bet = 15;
    let _r = transition(s, 4, 30);
}
#[test(should_fail_with = "dry side pot")]
fn no_dry_betting() {
    let mut s = example();
    s.can_respond = false;
    let _r = transition(s, 5, 0);
}
#[test(should_fail_with = "raise target")]
fn chip_creation() {
    let _r = transition(example(), 4, 101);
}
