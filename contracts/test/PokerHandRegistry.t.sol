// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PokerHandRegistry} from "../src/PokerHandRegistry.sol";
import {IHandVerifier} from "../src/IHandVerifier.sol";

// Test-only public-input binding double. This is NOT a cryptographic verifier.
// Never exported from src/ or used in a deployment script.
contract InputBindingDouble is IHandVerifier {
    function verify(bytes calldata proof, bytes32[] calldata inputs) external pure returns (bool) {
        if (inputs.length != 44) return false;
        return keccak256(proof) == keccak256(abi.encode(inputs));
    }
}
contract PokerHandRegistryTest {
    PokerHandRegistry registry;
    bytes32[9] players;
    uint64[9] initialStacks;
    uint64[9] finalStacks;
    bytes32 constant ID = keccak256("hand-1");
    bytes32 constant COMMIT = keccak256("commitments");

    function setUp() public {
        registry = new PokerHandRegistry(address(new InputBindingDouble()),address(42));
        for (uint160 i; i < 9; ++i) {players[i] = bytes32(uint256(i + 1)); initialStacks[i] = 100; finalStacks[i] = 100;}
        finalStacks[0] = 200; finalStacks[1] = 0;
    }
    function proof() internal view returns (bytes memory) {
        return abi.encode(registry.statement(ID, players, initialStacks, finalStacks, COMMIT, type(uint64).max));
    }
    function submit(bytes memory p) external {registry.verifyHand(ID, players, initialStacks, finalStacks, COMMIT, type(uint64).max, p);}
    function rejects(bytes memory p) internal {
        (bool ok,) = address(this).call(abi.encodeCall(this.submit, (p))); require(!ok,"accepted invalid hand");
        require(!registry.getHand(ID).exists,"failed call wrote result");
    }
    function testRecordsNinePlayers() public {
        registry.verifyHand(ID, players, initialStacks, finalStacks, COMMIT, type(uint64).max, proof());
        PokerHandRegistry.HandResult memory result = registry.getHand(ID);
        require(result.exists && result.initialStacks[1] == 100 && result.finalStacks[0] == 200);
    }
    function testRejectsDuplicateHand() public {
        bytes memory p=proof();registry.verifyHand(ID, players, initialStacks, finalStacks, COMMIT, type(uint64).max, p);
        (bool ok,) = address(this).call(abi.encodeCall(this.submit,(p)));require(!ok);
    }
    function testRejectsInvalidProof() public {rejects(hex"1234");}
    function testRejectsAlteredStacksWithSameTotal() public {bytes memory p=proof();finalStacks[0]--;finalStacks[2]++;rejects(p);}
    function testRejectsChipCreation() public {bytes memory p=proof();finalStacks[0]++;rejects(p);}
    function testRejectsPlayerSubstitution() public {bytes memory p=proof();players[2]=bytes32(uint256(99));rejects(p);}
    function testRejectsDuplicatePlayers() public {players[2]=players[1];rejects(proof());}
    function testRejectsGhostChips() public {players[8]=bytes32(0);rejects(proof());}
    function testRejectsZeroInitialStack() public {initialStacks[8]=0;finalStacks[8]=0;rejects(proof());}
    function testRejectsEOAVerifier() public {try new PokerHandRegistry(address(1234),address(42)) {revert("EOA accepted");} catch {}}
    function testDomainBindsRegistry() public {
        PokerHandRegistry other = new PokerHandRegistry(address(new InputBindingDouble()),address(42));
        require(registry.digest(ID,players,initialStacks,finalStacks,COMMIT, type(uint64).max)!=other.digest(ID,players,initialStacks,finalStacks,COMMIT, type(uint64).max));
    }
    function testFuzzConservation(uint64 amount) public {
        amount = uint64(uint256(amount) % 101);
        finalStacks[0] = 100 + amount; finalStacks[1] = 100 - amount;
        registry.verifyHand(ID,players,initialStacks,finalStacks,COMMIT, type(uint64).max,proof());
        require(registry.getHand(ID).exists);
    }
}
