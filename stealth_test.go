// Package stealthlab_test는 beatoz 체인에서 ERC-5564 스텔스 어카운트를 검증한다.
//
// # 테스트 시나리오
//
//  1. [배포] ERC5564Announcer + ERC6538Registry 컨트랙트 배포
//  2. [설정] Alice(수신자)가 SpendKey + ViewKey 생성 후 레지스트리에 등록
//  3. [송금] Bob(송신자)이 Alice의 스텔스 주소 파생 → BTZ 전송 + Announcement
//  4. [스캔] Alice가 Announcement를 스캔하여 자신의 수신 발견
//  5. [지출] Alice가 스텔스 개인키로 자산 지출 가능 여부 확인
package stealthlab_test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	stealth "github.com/beatoz/beatoz-labs/stealth-account/crypto"
	btztypes "github.com/beatoz/beatoz-go/types"
	"github.com/beatoz/beatoz-sdk-go/vm"
	btzweb3 "github.com/beatoz/beatoz-sdk-go/web3"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	abcitypes "github.com/tendermint/tendermint/abci/types"
)

// ─── 통합 테스트: 네트워크 필요 ──────────────────────────────────────────────

// TestStealthAccount는 beatoz 체인 전체 스텔스 어카운트 흐름을 검증한다.
func TestStealthAccount(t *testing.T) {
	setup(t)

	// 테스트 지갑 준비 (각 5 BTZ 펀딩)
	fundAmt := btztypes.ToGrans(5)
	deployer := newFundedWallet(t, "deployer", fundAmt)
	bob := newFundedWallet(t, "Bob(송신자)", fundAmt)
	alice := newFundedWallet(t, "Alice(수신자)", fundAmt) // 레지스트리 등록용

	// ── Step 1: 컨트랙트 배포 ──────────────────────────────────────────────────
	t.Log("=== Step 1: 컨트랙트 배포 ===")

	announcer, registry := deployContracts(t, deployer)
	t.Logf("  ERC5564Announcer: %X", announcer.GetAddress())
	t.Logf("  ERC6538Registry:  %X", registry.GetAddress())

	// ── Step 2: Alice 키 생성 + 메타-주소 등록 ────────────────────────────────
	t.Log("=== Step 2: Alice 메타-주소 등록 ===")

	aliceSpendKey, aliceViewKey := generateKeyPair(t), generateKeyPair(t)
	aliceMeta := &stealth.MetaAddress{
		SpendPubKey: &aliceSpendKey.PublicKey,
		ViewPubKey:  &aliceViewKey.PublicKey,
	}
	metaBytes := aliceMeta.Encode() // 66 bytes
	t.Logf("  Alice 메타-주소: %X", metaBytes)

	registerMetaAddress(t, alice, registry, metaBytes)
	t.Logf("  Alice (%X) 레지스트리 등록 완료", alice.Address())

	// ── Step 3: Bob이 레지스트리 조회 → 스텔스 주소 파생 ─────────────────────
	t.Log("=== Step 3: Bob이 스텔스 주소 파생 ===")

	retrieved := lookupMetaAddress(t, alice.Address(), registry)
	require_equal(t, hex.EncodeToString(metaBytes), hex.EncodeToString(retrieved), "메타-주소 일치")

	decodedMeta, err := stealth.DecodeMetaAddress(retrieved)
	require_no_error(t, err)

	ann, err := stealth.GenerateStealthAddress(decodedMeta)
	require_no_error(t, err)
	t.Logf("  스텔스 주소: %X", ann.StealthAddr)
	t.Logf("  임시 공개키: %X", ann.EphemeralKey)
	t.Logf("  뷰 태그:    0x%02X", ann.Metadata[1])

	// ── Step 4: Bob이 스텔스 주소로 BTZ 전송 + announce ─────────────────────
	t.Log("=== Step 4: Bob → 스텔스 주소로 BTZ 전송 ===")

	sendAmount := btztypes.ToGrans(2)
	sendAndAnnounce(t, bob, ann, sendAmount, announcer)

	// 스텔스 주소 잔액 확인
	stealthAcct, err := bzweb3.GetAccount(ann.StealthAddr)
	require_no_error(t, err)
	require_equal(t, sendAmount.Dec(), stealthAcct.Balance.Dec(), "스텔스 주소 잔액")
	t.Logf("  스텔스 주소 잔액: %s BTZ ✓", stealthAcct.Balance.Dec())

	// ── Step 5: Alice가 Announcement 스캔 ────────────────────────────────────
	t.Log("=== Step 5: Alice가 Announcement 스캔 ===")

	scanResult, err := stealth.Scan(ann, aliceViewKey, &aliceSpendKey.PublicKey, aliceSpendKey)
	require_no_error(t, err)
	require_not_nil(t, scanResult, "스캔 결과")
	require_equal(t, fmt.Sprintf("%X", ann.StealthAddr), fmt.Sprintf("%X", scanResult.StealthAddr), "스텔스 주소 일치")
	t.Logf("  수신 확인: %X ✓", scanResult.StealthAddr)

	// ── Step 6: Alice가 스텔스 개인키로 지출 ──────────────────────────────────
	t.Log("=== Step 6: Alice가 스텔스 개인키로 지출 ===")

	require_not_nil(t, scanResult.StealthPrivKey, "스텔스 개인키")

	// 스텔스 개인키로 지갑 생성 (ImportKey는 raw private key bytes를 받는다)
	stealthWallet := btzweb3.ImportKey(
		padLeft32(scanResult.StealthPrivKey.D.Bytes()),
		[]byte("stealth"),
	)
	require_no_error(t, stealthWallet.Unlock([]byte("stealth")))
	require_no_error(t, stealthWallet.SyncAccount(bzweb3))

	// 주소 일치 확인 (스텔스 키에서 파생된 주소 = 송금받은 스텔스 주소)
	require_equal(t,
		fmt.Sprintf("%X", ann.StealthAddr),
		fmt.Sprintf("%X", stealthWallet.Address()),
		"스텔스 지갑 주소 일치",
	)
	t.Logf("  스텔스 지갑 주소: %X ✓", stealthWallet.Address())

	// 스텔스 주소에서 다른 주소로 전송 (지출 가능성 검증)
	recipient := newFundedWallet(t, "수신자(임시)", uint256.NewInt(0))
	spendAmount := btztypes.ToGrans(1)

	spendRet, err := stealthWallet.TransferCommit(
		recipient.Address(), defGas(), defGasPrice(), spendAmount, bzweb3,
	)
	require_no_error(t, err)
	require_success(t, spendRet.CheckTx.Code, spendRet.CheckTx.Log)
	require_success(t, spendRet.DeliverTx.Code, spendRet.DeliverTx.Log)
	t.Logf("  스텔스 주소에서 %s BTZ 지출 성공 ✓", spendAmount.Dec())

	t.Log("=== 모든 단계 통과 — 스텔스 어카운트 PoC 성공 ===")
}

// ─── 단위 테스트: 크립토 로직 (네트워크 불필요) ──────────────────────────────

// TestStealthCrypto는 스텔스 크립토 연산을 노드 없이 검증한다.
func TestStealthCrypto(t *testing.T) {
	// Alice 키 생성
	spendKey := generateKeyPair(t)
	viewKey := generateKeyPair(t)

	meta := &stealth.MetaAddress{
		SpendPubKey: &spendKey.PublicKey,
		ViewPubKey:  &viewKey.PublicKey,
	}

	// 메타-주소 직렬화/역직렬화
	encoded := meta.Encode()
	require_equal(t, 66, len(encoded), "메타-주소 크기")

	decoded, err := stealth.DecodeMetaAddress(encoded)
	require_no_error(t, err)
	require_equal(t,
		hex.EncodeToString(ethcrypto.CompressPubkey(meta.SpendPubKey)),
		hex.EncodeToString(ethcrypto.CompressPubkey(decoded.SpendPubKey)),
		"SpendPubKey 직렬화 왕복",
	)
	require_equal(t,
		hex.EncodeToString(ethcrypto.CompressPubkey(meta.ViewPubKey)),
		hex.EncodeToString(ethcrypto.CompressPubkey(decoded.ViewPubKey)),
		"ViewPubKey 직렬화 왕복",
	)

	// Bob: 스텔스 주소 생성
	ann, err := stealth.GenerateStealthAddress(meta)
	require_no_error(t, err)
	require_true(t, len(ann.StealthAddr) == 20, "스텔스 주소 20바이트")
	require_true(t, len(ann.EphemeralKey) == 33, "임시 공개키 33바이트 compressed")
	require_true(t, ann.Metadata[0] == 0x01, "메타데이터 버전 0x01")
	t.Logf("스텔스 주소: %X", ann.StealthAddr)
	t.Logf("임시 공개키: %X", ann.EphemeralKey)
	t.Logf("뷰 태그:    0x%02X", ann.Metadata[1])

	// Alice: 스캔 (spend key 포함)
	result, err := stealth.Scan(ann, viewKey, &spendKey.PublicKey, spendKey)
	require_no_error(t, err)
	require_not_nil(t, result, "스캔 결과")
	require_equal(t,
		fmt.Sprintf("%X", ann.StealthAddr),
		fmt.Sprintf("%X", result.StealthAddr),
		"스캔 후 스텔스 주소 일치",
	)
	require_not_nil(t, result.StealthPrivKey, "스텔스 개인키 도출됨")

	// 스텔스 개인키로 지갑 생성 후 주소 검증
	stealthWallet := btzweb3.ImportKey(
		padLeft32(result.StealthPrivKey.D.Bytes()),
		[]byte("test"),
	)
	require_no_error(t, stealthWallet.Unlock([]byte("test")))
	require_equal(t,
		fmt.Sprintf("%X", ann.StealthAddr),
		fmt.Sprintf("%X", stealthWallet.Address()),
		"스텔스 키 → 지갑 주소 일치",
	)
	t.Log("스텔스 키 → 주소 파생 검증 ✓")

	// 뷰 태그 불일치 시 스킵 확인
	fakeAnn := &stealth.Announcement{
		SchemeID:     ann.SchemeID,
		StealthAddr:  ann.StealthAddr,
		EphemeralKey: ann.EphemeralKey,
		Metadata:     []byte{0x01, ann.Metadata[1] ^ 0xFF}, // 뷰 태그 반전
	}
	skipped, err := stealth.Scan(fakeAnn, viewKey, &spendKey.PublicKey, spendKey)
	require_no_error(t, err)
	require_equal(t, (*stealth.ScanResult)(nil), skipped, "뷰 태그 불일치 시 nil 반환")
	t.Log("뷰 태그 필터링 ✓")

	// 잘못된 임시 공개키는 에러가 아니라 무시되어야 한다.
	malformedAnn := &stealth.Announcement{
		SchemeID:     ann.SchemeID,
		StealthAddr:  ann.StealthAddr,
		EphemeralKey: []byte{0x02, 0x01},
		Metadata:     ann.Metadata,
	}
	ignored, err := stealth.Scan(malformedAnn, viewKey, &spendKey.PublicKey, spendKey)
	require_no_error(t, err)
	require_equal(t, (*stealth.ScanResult)(nil), ignored, "잘못된 announcement는 nil 반환")
	t.Log("잘못된 announcement 무시 ✓")

	// View-only 모드 (spendPrivKey = nil)
	viewOnly, err := stealth.Scan(ann, viewKey, &spendKey.PublicKey, nil)
	require_no_error(t, err)
	require_not_nil(t, viewOnly, "view-only 스캔 결과")
	require_equal(t, (*ecdsa.PrivateKey)(nil), viewOnly.StealthPrivKey, "view-only 모드 개인키 없음")
	t.Log("View-only 모드 ✓")

	// 타인의 키로 스캔 시 nil 반환
	strangerView := generateKeyPair(t)
	strangerSpend := generateKeyPair(t)
	stranger, err := stealth.Scan(ann, strangerView, &strangerSpend.PublicKey, strangerSpend)
	require_no_error(t, err)
	require_equal(t, (*stealth.ScanResult)(nil), stranger, "타인의 키로 스캔 시 nil")
	t.Log("타인 키 필터링 ✓")

	t.Log("크립토 로직 전체 검증 통과 ✓")
}

// ─── 단계별 헬퍼 ────────────────────────────────────────────────────────────

// deployContracts는 두 컨트랙트를 beatoz EVM에 배포한다.
func deployContracts(t *testing.T, deployer *btzweb3.Wallet) (*vm.EVMContract, *vm.EVMContract) {
	t.Helper()
	require_no_error(t, deployer.SyncAccount(bzweb3))

	announcer := deployContract(t, deployer, "./abis/ERC5564Announcer.json", "ERC5564Announcer", nil)
	registry := deployContract(t, deployer, "./abis/ERC6538Registry.json", "ERC6538Registry", nil)
	return announcer, registry
}

func deployContract(t *testing.T, deployer *btzweb3.Wallet, abiFile, name string, args []interface{}) *vm.EVMContract {
	t.Helper()
	c, err := vm.NewEVMContract(abiFile)
	require_no_error(t, err)

	ret, err := c.ExecCommit("", args,
		deployer, deployer.GetNonce(), contractGasLimit, defGasPrice(), uint256.NewInt(0), bzweb3)
	require_no_error(t, err)
	require_success(t, ret.CheckTx.Code, ret.CheckTx.Log)
	require_success(t, ret.DeliverTx.Code, ret.DeliverTx.Log)

	// DeliverTx.Data is a 32-byte tx hash on this testnet.
	// The actual 20-byte contract address is in the "evm" event attribute.
	contractAddr := contractAddrFromEvents(t, ret.DeliverTx.Events)
	c.SetAddress(contractAddr)
	require_not_nil(t, c.GetAddress(), name+" 주소")

	deployer.AddNonce()
	t.Logf("  %s 배포 완료: %X", name, c.GetAddress())
	return c
}

// contractAddrFromEvents는 DeliverTx 이벤트에서 EVM 컨트랙트 주소를 추출한다.
func contractAddrFromEvents(t *testing.T, events []abcitypes.Event) btztypes.Address {
	t.Helper()
	for _, evt := range events {
		if evt.Type != "evm" {
			continue
		}
		for _, attr := range evt.Attributes {
			if string(attr.Key) == "contractAddress" {
				addrHex := string(attr.Value) // lowercase hex, 40 chars
				b, err := hex.DecodeString(addrHex)
				require_no_error(t, err)
				require_true(t, len(b) == 20, "contractAddress는 20바이트")
				return btztypes.Address(b)
			}
		}
	}
	t.Fatal("이벤트에서 contractAddress를 찾을 수 없음")
	return nil
}

// registerMetaAddress는 Alice의 메타-주소를 ERC6538Registry에 등록한다.
func registerMetaAddress(t *testing.T, alice *btzweb3.Wallet, registry *vm.EVMContract, metaBytes []byte) {
	t.Helper()
	require_no_error(t, alice.SyncAccount(bzweb3))

	ret, err := registry.ExecCommit(
		"registerKeys",
		[]interface{}{
			new(big.Int).SetUint64(stealth.SchemeID), // schemeId (uint256)
			metaBytes,                                 // stealthMetaAddress (bytes, 66B)
		},
		alice, alice.GetNonce(), contractGasLimit, defGasPrice(), uint256.NewInt(0), bzweb3,
	)
	require_no_error(t, err)
	require_success(t, ret.CheckTx.Code, ret.CheckTx.Log)
	require_success(t, ret.DeliverTx.Code, ret.DeliverTx.Log)
	alice.AddNonce()
}

// lookupMetaAddress는 ERC6538Registry에서 registrant의 메타-주소를 조회한다.
func lookupMetaAddress(t *testing.T, registrant btztypes.Address, registry *vm.EVMContract) []byte {
	t.Helper()
	results, err := registry.Call(
		"stealthMetaAddressOf",
		[]interface{}{
			registrant.Array20(),                      // registrant (address)
			new(big.Int).SetUint64(stealth.SchemeID), // schemeId (uint256)
		},
		registrant, 0, bzweb3,
	)
	require_no_error(t, err)
	require_true(t, len(results) > 0, "stealthMetaAddressOf 반환값 있음")

	metaBytes, ok := results[0].([]byte)
	require_true(t, ok, "반환값 타입이 []byte")
	require_true(t, len(metaBytes) == 66, "메타-주소 66바이트")
	return metaBytes
}

// sendAndAnnounce는 스텔스 송금의 두 단계를 수행한다:
//  1. BTZ를 스텔스 주소로 TRX_TRANSFER
//  2. ERC5564Announcer에 announce() 호출
func sendAndAnnounce(t *testing.T, bob *btzweb3.Wallet, ann *stealth.Announcement, amount *uint256.Int, announcer *vm.EVMContract) {
	t.Helper()
	require_no_error(t, bob.SyncAccount(bzweb3))

	// 1) BTZ 전송
	transferRet, err := bob.TransferCommit(ann.StealthAddr, defGas(), defGasPrice(), amount, bzweb3)
	require_no_error(t, err)
	require_success(t, transferRet.CheckTx.Code, transferRet.CheckTx.Log)
	require_success(t, transferRet.DeliverTx.Code, transferRet.DeliverTx.Log)
	bob.AddNonce()
	t.Logf("  BTZ 전송 txHash: %X", transferRet.Hash)

	// 2) Announcement 발행
	// EVM address 파라미터는 *[20]byte 타입으로 전달
	ret, err := announcer.ExecCommit(
		"announce",
		[]interface{}{
			ann.SchemeID,            // schemeId (uint256)
			ann.StealthAddr.Array20(), // stealthAddress (address → *[20]byte)
			ann.EphemeralKey,        // ephemeralPubKey (bytes)
			ann.Metadata,            // metadata (bytes)
		},
		bob, bob.GetNonce(), contractGasLimit, defGasPrice(), uint256.NewInt(0), bzweb3,
	)
	require_no_error(t, err)
	require_success(t, ret.CheckTx.Code, ret.CheckTx.Log)
	require_success(t, ret.DeliverTx.Code, ret.DeliverTx.Log)
	bob.AddNonce()
	t.Logf("  Announcement txHash: %X", ret.Hash)
}

// ─── 유틸리티 ────────────────────────────────────────────────────────────────

// generateKeyPair는 새로운 secp256k1 키 쌍을 생성한다.
func generateKeyPair(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ethcrypto.GenerateKey()
	require_no_error(t, err)
	return key
}

// padLeft32는 슬라이스를 32바이트로 왼쪽 패딩한다.
// secp256k1 개인키 d 값이 32바이트 미만일 수 있기 때문이다.
func padLeft32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}
