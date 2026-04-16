// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title ERC6538Registry (Beatoz-adapted)
/// @notice 스텔스 메타-주소 등록소 (ERC-6538 기반)
/// @dev 수신자(Alice)가 스펜딩 공개키 + 뷰잉 공개키를 등록한다.
///      송신자(Bob)는 여기서 수신자의 메타-주소를 조회해 스텔스 주소를 파생한다.
contract ERC6538Registry {

    /// @notice 메타-주소가 등록/변경될 때 발행
    event StealthMetaAddressSet(
        address indexed registrant,
        uint256 indexed schemeId,
        bytes stealthMetaAddress
    );

    /// registrant => schemeId => stealthMetaAddress (66 bytes: spendPub || viewPub)
    mapping(address => mapping(uint256 => bytes)) private _registry;

    /// @notice 자신의 메타-주소를 등록한다
    /// @param schemeId           암호화 방식 ID (1 = secp256k1)
    /// @param stealthMetaAddress spendPubKey(33B) || viewPubKey(33B)
    function registerKeys(uint256 schemeId, bytes calldata stealthMetaAddress) external {
        require(stealthMetaAddress.length == 66, "ERC6538: meta-address must be 66 bytes");
        _registry[msg.sender][schemeId] = stealthMetaAddress;
        emit StealthMetaAddressSet(msg.sender, schemeId, stealthMetaAddress);
    }

    /// @notice 대리 등록 (서명 검증 생략 — PoC 용)
    function registerKeysOnBehalf(
        address registrant,
        uint256 schemeId,
        bytes calldata stealthMetaAddress
    ) external {
        require(stealthMetaAddress.length == 66, "ERC6538: meta-address must be 66 bytes");
        _registry[registrant][schemeId] = stealthMetaAddress;
        emit StealthMetaAddressSet(registrant, schemeId, stealthMetaAddress);
    }

    /// @notice 등록된 메타-주소 조회
    function stealthMetaAddressOf(
        address registrant,
        uint256 schemeId
    ) external view returns (bytes memory) {
        return _registry[registrant][schemeId];
    }
}
