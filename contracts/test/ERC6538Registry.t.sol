// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import {ERC6538Registry} from "../src/ERC6538Registry.sol";

interface Vm {
    function prank(address newSender) external;
    function expectRevert(bytes calldata revertData) external;
}

address constant VM_ADDRESS = address(uint160(uint256(keccak256("hevm cheat code"))));

contract ERC6538RegistryTest {
    Vm private constant VM = Vm(VM_ADDRESS);

    function testRegisterKeysOnBehalfRejectsThirdPartyOverwrite() public {
        ERC6538Registry registry = new ERC6538Registry();
        bytes memory metaAddress = hex"020102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F200302030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F2021";
        address registrant = address(0xA11CE);
        address attacker = address(0xBEEF);

        VM.prank(attacker);
        VM.expectRevert(bytes("ERC6538: unauthorized registrar"));
        registry.registerKeysOnBehalf(registrant, 1, metaAddress);
    }

    function testRegisterKeysOnBehalfAllowsRegistrant() public {
        ERC6538Registry registry = new ERC6538Registry();
        bytes memory metaAddress = hex"020102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F200302030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F2021";
        address registrant = address(0xA11CE);

        VM.prank(registrant);
        registry.registerKeysOnBehalf(registrant, 1, metaAddress);

        bytes memory stored = registry.stealthMetaAddressOf(registrant, 1);
        require(keccak256(stored) == keccak256(metaAddress), "meta-address mismatch");
    }
}
