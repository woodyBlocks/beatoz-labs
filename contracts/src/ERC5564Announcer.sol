// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title ERC5564Announcer (Beatoz-adapted)
/// @notice 스텔스 주소 Announcement 컨트랙트 (ERC-5564 기반)
/// @dev beatoz는 Tendermint-style 주소(RIPEMD160)를 사용하므로
///      stealthAddress 필드는 20-byte beatoz 주소를 담는다.
contract ERC5564Announcer {

    /// @notice 스텔스 송금 시 발행되는 이벤트
    /// @param schemeId     암호화 방식 ID (1 = secp256k1)
    /// @param stealthAddress 수신자의 일회용 스텔스 주소 (beatoz native)
    /// @param caller       announce() 호출자 (= 송신자)
    /// @param ephemeralPubKey 발신자가 생성한 임시 공개키 (33바이트, compressed)
    /// @param metadata     0x01 || viewTag(1B) — 수신자 스캔 최적화
    event Announcement(
        uint256 indexed schemeId,
        address indexed stealthAddress,
        address indexed caller,
        bytes ephemeralPubKey,
        bytes metadata
    );

    /// @notice 스텔스 송금을 블록체인에 알린다
    function announce(
        uint256 schemeId,
        address stealthAddress,
        bytes memory ephemeralPubKey,
        bytes memory metadata
    ) external {
        emit Announcement(schemeId, stealthAddress, msg.sender, ephemeralPubKey, metadata);
    }
}
