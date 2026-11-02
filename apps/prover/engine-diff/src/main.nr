use dep::poker_engine as engine;
fn main(
    config: engine::Config,
    deck: [u8; 52],
    action_count: u32,
    actions: [engine::Action; 64],
    expected: pub [u64; 9],
) {
    assert(action_count <= 64);
    dep::shuffle::assert_permutation(deck);
    let mut s = engine::initialize(config, deck);
    for i in 0..64 {
        if i < action_count {
            s = engine::apply(s, config, deck, actions[i]);
        }
    }
    assert(engine::settle(s, config) == expected, "differential result mismatch");
}
