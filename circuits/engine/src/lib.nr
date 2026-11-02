use dep::betting;
use dep::hand;
use dep::showdown;

pub struct Config {
    pub initial: [u64; 9],
    pub button: u32,
    pub small: u64,
    pub big: u64,
    pub ante: u64,
}
pub struct Action {
    pub seat: u32,
    pub kind: u8,
    pub amount: u64,
}
pub struct State {
    pub stack: [u64; 9],
    pub committed: [u64; 9],
    pub street_bet: [u64; 9],
    pub occupied: [bool; 9],
    pub folded: [bool; 9],
    pub acted: [bool; 9],
    pub last_bet: [u64; 9],
    pub holes: [[u8; 2]; 9],
    pub board: [u8; 5],
    pub board_len: u32,
    pub cursor: u32,
    pub street: u8,
    pub actor: u32,
    pub current: u64,
    pub min_raise: u64,
    pub refunds: [u64; 9],
}
fn live(s: State) -> [bool; 9] {
    let mut p = [false; 9];
    for i in 0..9 {
        p[i] = s.occupied[i] & !s.folded[i];
    }
    p
}
fn active(s: State) -> [bool; 9] {
    let mut p = live(s);
    for i in 0..9 {
        p[i] = p[i] & (s.stack[i] > 0);
    }
    p
}
fn count(flags: [bool; 9]) -> u32 {
    let mut n = 0;
    for i in 0..9 {
        if flags[i] {
            n = n + 1;
        }
    }
    n
}
fn next(from: u32, flags: [bool; 9]) -> u32 {
    let mut seat = 9;
    for offset in 1..10 {
        let i = (from + offset) % 9;
        if (seat == 9) & flags[i] {
            seat = i;
        }
    }
    seat
}
fn pay(mut s: State, seat: u32, amount: u64, street: bool) -> State {
    assert(amount <= s.stack[seat]);
    s.stack[seat] = s.stack[seat] - amount;
    s.committed[seat] = s.committed[seat] + amount;
    if street {
        s.street_bet[seat] = s.street_bet[seat] + amount;
    }
    s
}
fn minimum(a: u64, b: u64) -> u64 {
    if a < b {
        a
    } else {
        b
    }
}
fn refund(mut s: State) -> State {
    let mut first: u64 = 0;
    let mut second: u64 = 0;
    let mut seat = 9;
    for i in 0..9 {
        let c = s.committed[i];
        if c > first {
            second = first;
            first = c;
            seat = i;
        } else {
            if c > second {
                second = c;
            }
        }
    }
    if (seat < 9) & (first > second) {
        let d = first - second;
        s.committed[seat] = s.committed[seat] - d;
        s.stack[seat] = s.stack[seat] + d;
        s.street_bet[seat] = s.street_bet[seat] - minimum(d, s.street_bet[seat]);
        s.refunds[seat] = s.refunds[seat] + d;
    }
    s
}
pub fn initialize(c: Config, deck: [u8; 52]) -> State {
    assert(
        (c.button < 9)
            & (c.small > 0)
            & (c.big >= c.small)
            & (c.big <= 1000000000000)
            & (c.ante <= 1000000000000),
        "invalid configuration",
    );
    let mut occupied = [false; 9];
    for i in 0..9 {
        assert(c.initial[i] <= 1000000000000);
        occupied[i] = c.initial[i] > 0;
    }
    let n = count(occupied);
    assert((n >= 2) & occupied[c.button], "invalid roster");
    let mut s = State {
        stack: c.initial,
        committed: [0; 9],
        street_bet: [0; 9],
        occupied,
        folded: [false; 9],
        acted: [false; 9],
        last_bet: [0; 9],
        holes: [[0; 2]; 9],
        board: [0; 5],
        board_len: 0,
        cursor: 0,
        street: 0,
        actor: 9,
        current: c.big,
        min_raise: c.big,
        refunds: [0; 9],
    };
    for i in 0..9 {
        if occupied[i] {
            s = pay(s, i, minimum(c.ante, s.stack[i]), false);
        }
    }
    let sb = if n == 2 {
        c.button
    } else {
        next(c.button, occupied)
    };
    let bb = next(sb, occupied);
    s = pay(s, sb, minimum(c.small, s.stack[sb]), true);
    s = pay(s, bb, minimum(c.big, s.stack[bb]), true);
    for round in 0..2 {
        for offset in 1..10 {
            let seat = (c.button + offset) % 9;
            if occupied[seat] {
                s.holes[seat][round] = deck[s.cursor];
                s.cursor = s.cursor + 1;
            }
        }
    }
    advance(s, c, deck, bb)
}
fn advance(mut s: State, c: Config, deck: [u8; 52], mut from: u32) -> State {
    let mut done = false;
    for _iteration in 0..5 {
        if !done {
            if count(live(s)) == 1 {
                s = refund(s);
                s.street = 4;
                s.actor = 9;
                done = true;
            } else {
                let actors = active(s);
                let n = count(actors);
                if n == 1 {
                    let sole = next(8, actors);
                    let mut other: u64 = 0;
                    for i in 0..9 {
                        if (i != sole) & (s.street_bet[i] > other) {
                            other = s.street_bet[i];
                        }
                    }
                    s.current = other;
                    if s.street_bet[sole] < other {
                        s.actor = sole;
                        done = true;
                    }
                }
                if (n >= 2) & !done {
                    let mut needs = [false; 9];
                    for i in 0..9 {
                        needs[i] = actors[i] & (!s.acted[i] | (s.street_bet[i] < s.current));
                    }
                    let seat = next(from, needs);
                    if seat < 9 {
                        s.actor = seat;
                        done = true;
                    }
                }
                if !done {
                    s = refund(s);
                    if s.street == 3 {
                        s.street = 4;
                        s.actor = 9;
                        done = true;
                    } else {
                        s.cursor = s.cursor + 1; // burn
                        if s.street == 0 {
                            for j in 0..3 {
                                s.board[j] = deck[s.cursor];
                                s.cursor = s.cursor + 1;
                            }
                            s.board_len = 3;
                        } else {
                            s.board[s.board_len] = deck[s.cursor];
                            s.cursor = s.cursor + 1;
                            s.board_len = s.board_len + 1;
                        }
                        s.street = s.street + 1;
                        s.current = 0;
                        s.min_raise = c.big;
                        s.street_bet = [0; 9];
                        s.acted = [false; 9];
                        s.last_bet = [0; 9];
                        from = c.button;
                    }
                }
            }
        }
    }
    assert(done, "street advancement bound");
    s
}
pub fn apply(mut s: State, c: Config, deck: [u8; 52], a: Action) -> State {
    assert((s.street < 4) & (a.seat < 9) & (a.seat == s.actor), "turn order");
    let seat = a.seat;
    assert(active(s)[seat], "inactive actor");
    let local = betting::State {
        stack: s.stack[seat],
        street_bet: s.street_bet[seat],
        current_bet: s.current,
        min_raise: s.min_raise,
        acted: s.acted[seat],
        last_bet: s.last_bet[seat],
        can_respond: count(active(s)) >= 2,
    };
    let r = betting::transition(local, a.kind, a.amount);
    let paid = s.stack[seat] - r.stack;
    s.stack[seat] = r.stack;
    s.committed[seat] = s.committed[seat] + paid;
    s.street_bet[seat] = r.street_bet;
    s.current = r.current_bet;
    s.min_raise = r.min_raise;
    s.folded[seat] = r.folded;
    s.acted[seat] = true;
    s.last_bet[seat] = s.current;
    s = advance(s, c, deck, seat);
    let mut total: u64 = 0;
    let mut initial: u64 = 0;
    for i in 0..9 {
        assert(s.street_bet[i] <= s.committed[i]);
        total = total + s.stack[i] + s.committed[i];
        initial = initial + c.initial[i];
    }
    assert(total == initial, "transition conservation");
    s
}
pub fn settle(mut s: State, c: Config) -> [u64; 9] {
    assert(s.street == 4, "hand incomplete");
    s = refund(s);
    let eligible = live(s);
    let n = count(eligible);
    let mut ranks: [u32; 9] = [0; 9];
    if n > 1 {
        assert(s.board_len == 5);
        for i in 0..9 {
            if eligible[i] {
                ranks[i] = showdown::evaluate7([
                    s.holes[i][0],
                    s.holes[i][1],
                    s.board[0],
                    s.board[1],
                    s.board[2],
                    s.board[3],
                    s.board[4],
                ]);
            }
        }
    }
    let payouts = hand::distribute(s.committed, eligible, ranks, c.button);
    let mut final_stacks = [0; 9];
    for i in 0..9 {
        final_stacks[i] = s.stack[i] + payouts[i];
    }
    hand::assert_conservation(c.initial, final_stacks);
    final_stacks
}
fn identity() -> [u8; 52] {
    let mut d = [0; 52];
    for i in 0..52 {
        d[i] = i as u8;
    }
    d
}
#[test]
fn nine_all_in() {
    let d = identity();
    let c = Config {
        initial: [100, 70, 50, 100, 35, 100, 80, 100, 60],
        button: 0,
        small: 5,
        big: 10,
        ante: 1,
    };
    let mut s = initialize(c, d);
    for i in 0..9 {
        let kind = if i == 0 { 5 } else { 2 };
        s = apply(s, c, d, Action { seat: s.actor, kind, amount: 0 });
    }
    let _f = settle(s, c);
}
#[test]
fn heads_up_call_check() {
    let d = identity();
    let c =
        Config { initial: [100, 100, 0, 0, 0, 0, 0, 0, 0], button: 0, small: 5, big: 10, ante: 0 };
    let mut s = initialize(c, d);
    assert(s.actor == 0);
    s = apply(s, c, d, Action { seat: 0, kind: 2, amount: 0 });
    s = apply(s, c, d, Action { seat: 1, kind: 1, amount: 0 });
    assert((s.street == 1) & (s.actor == 1));
}
#[test(should_fail_with = "turn order")]
fn wrong_turn() {
    let d = identity();
    let c = Config { initial: [100; 9], button: 0, small: 5, big: 10, ante: 0 };
    let s = initialize(c, d);
    let _s = apply(s, c, d, Action { seat: 0, kind: 2, amount: 0 });
}

fn be40(v: u64) -> [u8; 5] {
    assert(v < 1099511627776);
    let b = dep::crypto::be64(v);
    [b[3], b[4], b[5], b[6], b[7]]
}
pub fn view(s: State, sequence: u32) -> [u8; 32] {
    assert((s.street < 4) & (s.actor < 9) & (sequence < 64));
    let mut players = [0; 90];
    let mut folded: u16 = 0;
    for i in 0..9 {
        let a = be40(s.stack[i]); let b = be40(s.street_bet[i]);
        for j in 0..5 { players[i*10+j] = a[j]; players[i*10+5+j] = b[j]; }
        if s.folded[i] { folded = folded | ((1 as u16) << (i as u16)); }
    }
    dep::crypto::sha("ZKPVIEW3".as_bytes().concat(players)
        .concat([(folded >> 8) as u8, folded as u8])
        .concat(be40(s.current)).concat(be40(s.min_raise)).concat(s.board)
        .concat([s.street, s.actor as u8, sequence as u8]))
}
