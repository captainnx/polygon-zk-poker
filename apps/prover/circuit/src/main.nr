// Toolchain smoke circuit: proves ONLY a 7-card rank, not a poker hand.
fn main(cards: [u8; 7], expected_rank: pub u32) {
    assert(showdown::evaluate7(cards) == expected_rank, "incorrect claimed rank");
}

#[test]
fn valid() {
    main([32, 36, 40, 44, 48, 1, 6], 6783750);
}
#[test(should_fail_with = "incorrect claimed rank")]
fn invalid_rank() {
    main([32, 36, 40, 44, 48, 1, 6], 6783749);
}
#[test(should_fail_with = "duplicate card")]
fn invalid_cards() {
    main([32, 32, 40, 44, 48, 1, 6], 6783750);
}
