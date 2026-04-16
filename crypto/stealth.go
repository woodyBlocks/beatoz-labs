// Package stealth은 ERC-5564 기반 스텔스 어드레스 크립토를 구현한다.
//
// # beatoz 적응 사항
//
// 표준 ERC-5564는 Ethereum-style 주소(keccak256)를 사용하지만,
// beatoz는 Tendermint-style 주소(RIPEMD160(SHA256(pubkey)))를 사용한다.
// 이 패키지는 주소 파생만 beatoz-native 방식으로 변경하고,
// 나머지 암호화 로직(ECDH, HKDF, 키 파생)은 ERC-5564와 동일하게 유지한다.
//
// # 흐름
//
//  1. Alice(수신자)는 SpendKey + ViewKey 두 쌍을 생성하고 MetaAddress로 등록
//  2. Bob(송신자)은 Alice의 MetaAddress로 일회용 StealthAddress를 파생
//  3. Bob은 ERC5564Announcer에 (stealthAddr, ephemeralPubKey, metadata)를 announce
//  4. Bob은 stealthAddr로 BTZ 전송
//  5. Alice는 Announcement 이벤트를 스캔하여 자신의 수신 확인
//  6. Alice는 StealthPrivKey를 파생하여 자산 지출
package stealth

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	btztypes "github.com/beatoz/beatoz-go/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	tmsecp256k1 "github.com/tendermint/tendermint/crypto/secp256k1"
)

// SchemeID는 secp256k1 기반 스텔스 주소 방식의 식별자다 (ERC-5564 §5).
const SchemeID = uint64(1)

// MetaAddress는 수신자의 스텔스 메타-주소다.
// ERC-6538 레지스트리에 등록되어 송신자가 조회한다.
type MetaAddress struct {
	SpendPubKey *ecdsa.PublicKey // 자산 지출용 공개키
	ViewPubKey  *ecdsa.PublicKey // 수신 스캔용 공개키 (감사 위임 가능)
}

// Encode는 메타-주소를 66바이트로 직렬화한다: spendPub(33B) || viewPub(33B)
func (m *MetaAddress) Encode() []byte {
	spend := ethcrypto.CompressPubkey(m.SpendPubKey)
	view := ethcrypto.CompressPubkey(m.ViewPubKey)
	return append(spend, view...)
}

// DecodeMetaAddress는 66바이트 슬라이스를 MetaAddress로 역직렬화한다.
func DecodeMetaAddress(b []byte) (*MetaAddress, error) {
	if len(b) != 66 {
		return nil, fmt.Errorf("stealth: meta-address must be 66 bytes, got %d", len(b))
	}
	spendPub, err := ethcrypto.DecompressPubkey(b[:33])
	if err != nil {
		return nil, fmt.Errorf("stealth: decompress spend key: %w", err)
	}
	viewPub, err := ethcrypto.DecompressPubkey(b[33:])
	if err != nil {
		return nil, fmt.Errorf("stealth: decompress view key: %w", err)
	}
	return &MetaAddress{SpendPubKey: spendPub, ViewPubKey: viewPub}, nil
}

// Announcement는 ERC5564Announcer 컨트랙트에 전달할 데이터를 담는다.
type Announcement struct {
	SchemeID     *big.Int       // 방식 ID (=1)
	StealthAddr  btztypes.Address // beatoz-native 스텔스 주소 (20B, RIPEMD160)
	EphemeralKey []byte           // 임시 공개키 (33B, compressed secp256k1)
	Metadata     []byte           // 0x01 || viewTag(1B)
}

// ViewTag는 Announcement 메타데이터에서 뷰 태그를 추출한다.
// 뷰 태그가 없으면 false를 반환한다.
func (a *Announcement) ViewTag() (byte, bool) {
	if len(a.Metadata) >= 2 && a.Metadata[0] == 0x01 {
		return a.Metadata[1], true
	}
	return 0, false
}

// GenerateStealthAddress는 수신자의 MetaAddress로 일회용 스텔스 주소를 파생한다.
// 송신자(Bob)가 호출한다.
//
// 알고리즘 (ERC-5564 §4.1 secp256k1):
//  1. 임시 키 쌍 생성: r ∈ ℤ_n, R = r·G
//  2. ECDH: S_x = x(r · P_view)  (공유 비밀 = x좌표)
//  3. s_h = keccak256(S_x)
//  4. viewTag = s_h[0]
//  5. P_stealth = P_spend + s_h·G
//  6. addr_stealth = beatozAddr(P_stealth)   ← beatoz 적응점
func GenerateStealthAddress(meta *MetaAddress) (*Announcement, error) {
	// 1. 임시 키 쌍
	r, err := ethcrypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("stealth: generate ephemeral key: %w", err)
	}

	// 2. ECDH (x좌표)
	sx := ecdhX(r, meta.ViewPubKey)

	// 3. 해시
	sHash := ethcrypto.Keccak256(sx)

	// 4. 스텔스 공개키: P_stealth = P_spend + sHash·G
	stealthPub, err := addTweak(meta.SpendPubKey, sHash)
	if err != nil {
		return nil, fmt.Errorf("stealth: add tweak: %w", err)
	}

	// 5. beatoz-native 주소 파생 (RIPEMD160)
	stealthAddr := pubKeyToAddr(stealthPub)

	// 6. 임시 공개키 직렬화 (compressed 33B)
	ephemKey := ethcrypto.CompressPubkey(&r.PublicKey)

	// 7. 메타데이터: version(0x01) || viewTag
	metadata := []byte{0x01, sHash[0]}

	return &Announcement{
		SchemeID:     new(big.Int).SetUint64(SchemeID),
		StealthAddr:  stealthAddr,
		EphemeralKey: ephemKey,
		Metadata:     metadata,
	}, nil
}

// ScanResult는 수신자 스캔 성공 시 반환된다.
type ScanResult struct {
	StealthAddr    btztypes.Address
	StealthPrivKey *ecdsa.PrivateKey // view-only 모드라면 nil
}

// Scan은 Announcement 이벤트가 자신에게 온 것인지 확인한다.
// 수신자(Alice)가 호출한다.
//
// viewPrivKey는 항상 필요하다.
// spendPrivKey가 nil이면 view-only 모드로, StealthPrivKey를 파생하지 않는다.
//
// 반환값이 nil이면 자신의 수신이 아님 (= 스킵).
//
// 알고리즘:
//  1. ECDH: S_x = x(p_view · R)
//  2. s_h = keccak256(S_x)
//  3. viewTag 일치 확인 (불일치 시 즉시 nil 반환 — 99.6% 스킵)
//  4. P_stealth = P_spend + s_h·G → addr 파생 후 비교
//  5. (선택) p_stealth = p_spend + s_h  (mod n)
func Scan(
	ann *Announcement,
	viewPrivKey *ecdsa.PrivateKey,
	spendPubKey *ecdsa.PublicKey,
	spendPrivKey *ecdsa.PrivateKey, // nil = view-only
) (*ScanResult, error) {
	if ann.SchemeID.Uint64() != SchemeID {
		return nil, nil
	}

	// 1. 임시 공개키 복원
	R, err := ethcrypto.DecompressPubkey(ann.EphemeralKey)
	if err != nil {
		return nil, nil // malformed announcement는 무시
	}

	// 2. ECDH
	sx := ecdhX(viewPrivKey, R)

	// 3. 해시
	sHash := ethcrypto.Keccak256(sx)

	// 4. 뷰 태그 확인 (빠른 필터링)
	if vt, ok := ann.ViewTag(); ok && vt != sHash[0] {
		return nil, nil // 내 것 아님
	}

	// 5. 스텔스 주소 비교
	stealthPub, err := addTweak(spendPubKey, sHash)
	if err != nil {
		return nil, fmt.Errorf("stealth: add tweak: %w", err)
	}
	derived := pubKeyToAddr(stealthPub)
	if string(derived) != string(ann.StealthAddr) {
		return nil, nil // 뷰 태그 충돌(false positive) 또는 내 것 아님
	}

	// 6. 스텔스 개인키 파생
	result := &ScanResult{StealthAddr: ann.StealthAddr}
	if spendPrivKey != nil {
		stealthPriv, err := deriveStealthKey(spendPrivKey, sHash, stealthPub)
		if err != nil {
			return nil, fmt.Errorf("stealth: derive stealth key: %w", err)
		}
		result.StealthPrivKey = stealthPriv
	}

	return result, nil
}

// ─── 내부 헬퍼 ────────────────────────────────────────────────────────────────

// ecdhX는 secp256k1 ECDH를 수행하고 공유 점의 x좌표(32B)를 반환한다.
func ecdhX(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) []byte {
	curve := ethcrypto.S256()
	x, _ := curve.ScalarMult(pub.X, pub.Y, priv.D.Bytes())
	buf := make([]byte, 32)
	x.FillBytes(buf)
	return buf
}

// addTweak는 P + tweak·G 를 계산한다.
func addTweak(pub *ecdsa.PublicKey, tweak []byte) (*ecdsa.PublicKey, error) {
	curve := ethcrypto.S256()
	// tweak·G
	tx, ty := curve.ScalarBaseMult(tweak)
	if tx == nil {
		return nil, errors.New("stealth: scalar base mult failed")
	}
	// P + tweak·G
	rx, ry := curve.Add(pub.X, pub.Y, tx, ty)
	return &ecdsa.PublicKey{Curve: curve, X: rx, Y: ry}, nil
}

// deriveStealthKey는 p_stealth = p_spend + sHash (mod n)을 계산한다.
func deriveStealthKey(spendPriv *ecdsa.PrivateKey, sHash []byte, stealthPub *ecdsa.PublicKey) (*ecdsa.PrivateKey, error) {
	n := ethcrypto.S256().Params().N
	d := new(big.Int).Add(spendPriv.D, new(big.Int).SetBytes(sHash))
	d.Mod(d, n)
	if d.Sign() == 0 {
		return nil, errors.New("stealth: derived private key is zero — retry with different ephemeral key")
	}
	return &ecdsa.PrivateKey{
		PublicKey: *stealthPub,
		D:         d,
	}, nil
}

// pubKeyToAddr는 secp256k1 공개키를 beatoz-native 주소로 변환한다.
// beatoz는 Tendermint-style 주소(RIPEMD160(SHA256(compressed_pubkey)))를 사용한다.
func pubKeyToAddr(pub *ecdsa.PublicKey) btztypes.Address {
	compressed := ethcrypto.CompressPubkey(pub)
	return btztypes.Address(tmsecp256k1.PubKey(compressed).Address())
}
