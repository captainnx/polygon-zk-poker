// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PokerHandRegistry} from "../src/PokerHandRegistry.sol";

interface Vm {
    function envAddress(string calldata name) external returns (address);
    function envBool(string calldata name) external returns (bool);
    function startBroadcast() external;
    function stopBroadcast() external;
}
contract Deploy {
    Vm constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));
    function run() external returns (PokerHandRegistry registry) {
        // Test deployments are experimental; this flag is not a claim of audit.
        if(block.chainid == 80002) require(vm.envBool("ALLOW_EXPERIMENTAL_AMOY"), "experimental Amoy acknowledgement required");
        else { require(block.chainid == 137, "unsupported chain"); require(vm.envBool("ALLOW_MAINNET_DEPLOYMENT"), "mainnet explicitly disabled by default"); }
        address verifier = vm.envAddress("HAND_VERIFIER_ADDRESS");
        vm.startBroadcast();registry = new PokerHandRegistry(verifier,vm.envAddress("AUTH_ISSUER_ADDRESS"));vm.stopBroadcast();
    }
}
