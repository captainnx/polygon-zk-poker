// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IHandVerifier} from "./IHandVerifier.sol";

/// @notice Records a bounded signed hand after its Noir/UltraHonk proof verifies.
/// Development release; requires independent circuit and protocol review.
contract PokerHandRegistry {
    uint256 public constant MAX_INITIAL_STACK = 1_000_000_000_000;
    bytes32 public constant PROTOCOL_VERSION = keccak256("polygon-zk-poker/registry/v3");
    IHandVerifier public immutable verifier;
 address public immutable authIssuer;

    struct HandResult {
        bytes32[9] players;
        uint64[9] initialStacks;
        uint64[9] finalStacks;
        bytes32 protocolCommitment;
        bytes32 resultDigest;
        uint64 validUntil;
        bool exists;
    }
    mapping(bytes32 => HandResult) private results;
    event HandVerified(bytes32 indexed handId, bytes32 indexed resultDigest, bytes32 protocolCommitment);
    error InvalidVerifier();
    error InvalidHand();
    error AlreadyVerified();
    error InvalidPlayers();
    error InvalidChips();
    error InvalidProof();

    constructor(address verifier_,address issuer_) {
        if (verifier_.code.length == 0) revert InvalidVerifier();
        if(issuer_==address(0))revert InvalidPlayers();
 authIssuer=issuer_;verifier = IHandVerifier(verifier_);
    }

    /// @dev The emitted digest is an index; proof binding uses statement().
    function digest(
        bytes32 handId, bytes32[9] calldata players, uint64[9] calldata initialStacks,
        uint64[9] calldata finalStacks, bytes32 protocolCommitment, uint64 validUntil
    ) public view returns (bytes32) {
        return keccak256(abi.encode(PROTOCOL_VERSION, block.chainid, address(this), handId, players, initialStacks, finalStacks, protocolCommitment, validUntil,authIssuer));
    }

    function verifyHand(
        bytes32 handId, bytes32[9] calldata players, uint64[9] calldata initialStacks,
        uint64[9] calldata finalStacks, bytes32 protocolCommitment, uint64 validUntil, bytes calldata proof
    ) external {
        if (handId == bytes32(0) || protocolCommitment == bytes32(0) || block.timestamp >= validUntil) revert InvalidHand();
        if (results[handId].exists) revert AlreadyVerified();
        uint256 count;
        uint256 beforeTotal;
        uint256 afterTotal;
        for (uint256 i; i < 9; ++i) {
            if (players[i] == bytes32(0)) {
                if (initialStacks[i] != 0 || finalStacks[i] != 0) revert InvalidPlayers();
            } else {
                ++count;
                if (initialStacks[i] == 0 || initialStacks[i] > MAX_INITIAL_STACK || finalStacks[i] > 9 * MAX_INITIAL_STACK) revert InvalidChips();
                for (uint256 j; j < i; ++j) if (players[i] == players[j]) revert InvalidPlayers();
            }
            beforeTotal += initialStacks[i];
            afterTotal += finalStacks[i];
        }
        if (count < 2) revert InvalidPlayers();
        if (beforeTotal != afterTotal) revert InvalidChips();
        bytes32 d = digest(handId, players, initialStacks, finalStacks, protocolCommitment, validUntil);
        bytes32[] memory inputs = statement(handId, players, initialStacks, finalStacks, protocolCommitment, validUntil);
        // STATICCALL because verify() is view; a verifier cannot reenter to write.
        if (!verifier.verify(proof, inputs)) revert InvalidProof();
        results[handId] = HandResult(players, initialStacks, finalStacks, protocolCommitment, d, validUntil, true);
        emit HandVerified(handId, d, protocolCommitment);
    }

    /// @dev Every input fits the BN254 scalar field without modular reduction.
    function statement(
        bytes32 handId, bytes32[9] calldata players, uint64[9] calldata initialStacks,
        uint64[9] calldata finalStacks, bytes32 protocolCommitment, uint64 validUntil
    ) public view returns (bytes32[] memory inputs) {
        inputs = new bytes32[](44);
        inputs[0] = bytes32(block.chainid);
        inputs[1] = bytes32(uint256(uint160(address(this))));
        inputs[2] = bytes32(uint256(handId) >> 128);
        inputs[3] = bytes32(uint256(uint128(uint256(handId))));
        for (uint256 i; i < 9; ++i) {
            inputs[4+2*i] = bytes32(uint256(players[i]) >> 128);
 inputs[5+2*i]=bytes32(uint256(uint128(uint256(players[i]))));
            inputs[22+i] = bytes32(uint256(initialStacks[i]));
            inputs[31+i] = bytes32(uint256(finalStacks[i]));
        }
        inputs[40] = bytes32(uint256(protocolCommitment) >> 128);
        inputs[41] = bytes32(uint256(uint128(uint256(protocolCommitment))));
        inputs[42] = bytes32(uint256(validUntil));
 inputs[43]=bytes32(uint256(uint160(authIssuer)));
    }

    function getHand(bytes32 handId) external view returns (HandResult memory) { return results[handId]; }
}
