// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import {ERC5564Announcer} from "../src/ERC5564Announcer.sol";

interface Vm {
    function expectRevert(bytes calldata revertData) external;
}

address constant VM_ADDRESS = address(uint160(uint256(keccak256("hevm cheat code"))));

contract ERC5564AnnouncerTest {
    Vm private constant VM = Vm(VM_ADDRESS);

    function testAnnounceRejectsMalformedEphemeralKeyLength() public {
        ERC5564Announcer announcer = new ERC5564Announcer();

        VM.expectRevert(bytes("ERC5564: ephemeral key must be 33 bytes"));
        announcer.announce(1, address(0x1234), hex"0201", hex"01AA");
    }

    function testAnnounceRejectsMalformedMetadata() public {
        ERC5564Announcer announcer = new ERC5564Announcer();
        bytes memory ephemeralKey = hex"020102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20";

        VM.expectRevert(bytes("ERC5564: metadata must be 0x01 || viewTag"));
        announcer.announce(1, address(0x1234), ephemeralKey, hex"010203");
    }

    function testAnnounceAcceptsCanonicalAnnouncement() public {
        ERC5564Announcer announcer = new ERC5564Announcer();
        bytes memory ephemeralKey = hex"020102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20";

        announcer.announce(1, address(0x1234), ephemeralKey, hex"01AA");
    }
}
