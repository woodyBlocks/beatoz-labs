# Stealth Account PoC on beatoz

ERC-5564 스텔스 어드레스 + ERC-6538 메타-주소 레지스트리를 beatoz 체인에 적용한 PoC입니다.

---

## 개요

### 스텔스 어카운트란?

일반 블록체인 송금은 송신자·수신자 주소가 모두 온체인에 노출됩니다.

```
Bob(CD17D7CC) → Alice(495AD748)  송금
→ 누구든 블록체인을 조회하면 "Bob이 Alice에게 보냈다"는 사실을 알 수 있음
```

스텔스 어카운트는 **수신자 프라이버시**를 보호합니다.

```
Bob(CD17D7CC) → 스텔스 주소(F5775178)  송금
→ 블록체인 관찰자는 F5775178이 Alice의 주소임을 알 수 없음
→ Alice만 ECDH 연산으로 자신의 수신임을 확인하고 자금 인출 가능
```

### beatoz 적응 사항

표준 ERC-5564는 Ethereum 주소 체계(keccak256)를 사용하지만, beatoz는 Tendermint 주소 체계(RIPEMD160(SHA256(pubkey)))를 사용합니다. 주소 파생 방식만 beatoz-native로 변경하고 나머지 암호화 로직(ECDH, HKDF, 키 파생)은 ERC-5564 표준과 동일하게 유지합니다.

---

## 아키텍처

```
beatoz-labs/stealth-account/
├── contracts/                  Solidity 컨트랙트 (Foundry 프로젝트)
│   └── src/
│       ├── ERC5564Announcer.sol    스텔스 송금 공표 컨트랙트
│       └── ERC6538Registry.sol    메타-주소 레지스트리 컨트랙트
├── abis/                       컴파일된 ABI (make build-contracts 생성)
│   ├── ERC5564Announcer.json
│   └── ERC6538Registry.json
├── crypto/
│   └── stealth.go              ERC-5564 핵심 암호화 로직 (beatoz 적응)
├── cmd/mkwallet/               테스트용 펀더 지갑 생성 도구
├── testdata/                   펀더 지갑 파일 저장 디렉토리
├── setup_test.go               테스트 공통 설정 및 헬퍼
├── stealth_test.go             통합 테스트 + 크립토 단위 테스트
├── Makefile
└── go.mod
```

---

## 컨트랙트

### ERC6538Registry

수신자가 자신의 공개키 묶음(메타-주소)을 온체인에 등록하는 저장소입니다.

```solidity
// 66바이트 메타-주소 등록 (SpendPubKey 33B + ViewPubKey 33B)
function registerKeys(uint256 schemeId, bytes calldata stealthMetaAddress) external

// 등록된 메타-주소 조회
function stealthMetaAddressOf(address registrant, uint256 schemeId) external view returns (bytes memory)
```

### ERC5564Announcer

송신자가 스텔스 송금 사실을 이벤트로 공표합니다. 수신자는 이 이벤트를 스캔하여 자신의 수신분을 찾습니다.

```solidity
// 스텔스 송금 공표
function announce(
    uint256 schemeId,
    address stealthAddress,
    bytes calldata ephemeralPubKey,
    bytes calldata metadata
) external
```

---

## 암호화 흐름

### 스텔스 주소 생성 (송신자 Bob)

```
1. 임시 키쌍 생성:  r (랜덤), R = r·G  (이 송금에만 사용 후 폐기)
2. ECDH:           S_x = x좌표( r × Alice_ViewPubKey )
3. 해시:           s_h = keccak256(S_x)
                   뷰 태그 = s_h[0]  (스캔 최적화용 1바이트)
4. 스텔스 공개키:  P_stealth = Alice_SpendPubKey + s_h·G
5. 주소 파생:      RIPEMD160(SHA256(P_stealth))  ← beatoz-native
```

### 수신 스캔 (수신자 Alice)

```
1. 이벤트에서 임시 공개키 R 추출
2. ECDH:         S_x = x좌표( Alice_ViewPrivKey × R )  → Bob과 동일한 값
3. 해시:         s_h = keccak256(S_x)
4. 뷰 태그 비교: s_h[0] ≠ 이벤트 뷰 태그 → 즉시 스킵 (99.6% 필터링)
5. 주소 검증:    P_stealth 재계산 → 주소 일치 여부 확인
6. 키 파생:      p_stealth = Alice_SpendPrivKey + s_h (mod N)
```

---

## 테스트 결과 (beatoz testnet)

### 테스트 환경

| 항목 | 값 |
|------|----|
| 네트워크 | beatoz testnet |
| Chain ID | `0xbea701` |
| RPC | `https://rpc-testnet0.beatoz.io` |

### 실행 결과

```
--- PASS: TestStealthAccount (12.02s)
```

### 실제 주소 및 트랜잭션

| 역할 | 주소 |
|------|------|
| Deployer | `48B33D9374386E387A4A36647E87F711E6C53821` |
| Bob (송신자) | `CD17D7CC2151E3913B1CF9A25C5C6708A7D70871` |
| Alice (수신자) | `495AD7481E92880626BB8B9F0E3B51035C0ED8BC` |
| ERC5564Announcer | `1E20F2CA14A34647A5E69189A95D9B038DB7870F` |
| ERC6538Registry | `D6A7850EC1C8FF1C4EFFC4C4E1C38526A3C463E7` |
| **스텔스 주소** | `F5775178A0640C262D3B3169CC069E9B2957C7E6` |

| 단계 | TxHash |
|------|--------|
| Bob → 스텔스 주소 (2 BTZ 입금) | `5A6CDC14F75462C6A02DDFE7BB1CB9CD2B175977B560B7875CA6F8A665F8BE99` |
| Announce | `5CB05E9192DD2D9A27EED706D5F485830A26B8264DC6EF152C33D46E5374FF93` |

### 단계별 검증

| 단계 | 내용 | 결과 |
|------|------|------|
| Step 1 | ERC5564Announcer + ERC6538Registry 컨트랙트 배포 | ✅ |
| Step 2 | Alice 메타-주소 레지스트리 등록 | ✅ |
| Step 3 | Bob이 스텔스 주소 파생 (뷰 태그 `0xB8`) | ✅ |
| Step 4 | Bob → 스텔스 주소 2 BTZ 입금 + Announce | ✅ |
| Step 5 | Alice 수신 스캔 — 스텔스 주소 확인 | ✅ |
| Step 6 | Alice 스텔스 개인키로 1 BTZ 인출 성공 | ✅ |

---

## 실행 방법

### 사전 요구사항

```bash
# Foundry 설치 (Solidity 컴파일)
curl -L https://foundry.paradigm.xyz | bash
foundryup

# Go 1.23+
go version
```

### 1. 컨트랙트 컴파일

```bash
make build-contracts
```

### 2. 펀더 지갑 생성

```bash
make funder
# → testdata/funder.json 생성
# → 출력된 주소에 beatoz 노드에서 충분한 BTZ 전송 필요 (최소 20 BTZ 권장)
```

### 3. 크립토 단위 테스트 (노드 불필요)

```bash
make test-crypto
```

네트워크 없이 암호화 로직만 검증합니다.

```
TestStealthCrypto:
  - 메타-주소 인코딩/디코딩
  - 스텔스 주소 생성
  - 뷰 태그 필터링
  - View-only 모드
  - 타인 키 거부
  - 스텔스 키 → 지갑 주소 일치
```

### 4. 통합 테스트 (beatoz 노드 필요)

```bash
# 기본값: testnet
make test

# 환경 변수로 오버라이드 가능
BEATOZ_RPC_URL=https://rpc-testnet0.beatoz.io \
BEATOZ_WS_URL=wss://rpc-testnet0.beatoz.io/websocket \
BEATOZ_FUNDER_KEY=./testdata/funder.json \
BEATOZ_FUNDER_PASS=1111 \
go test -v -run TestStealthAccount -timeout 180s ./...
```

### 환경 변수

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `BEATOZ_RPC_URL` | `https://rpc-testnet0.beatoz.io` | beatoz 노드 RPC |
| `BEATOZ_WS_URL` | `wss://rpc-testnet0.beatoz.io/websocket` | WebSocket URL |
| `BEATOZ_FUNDER_KEY` | `./testdata/funder.json` | 펀더 지갑 파일 경로 |
| `BEATOZ_FUNDER_PASS` | `1111` | 펀더 지갑 비밀번호 |

---

## 한계 및 다음 과제

### 현재 보호 범위

| 항목 | 상태 |
|------|------|
| 수신자 프라이버시 (누가 받았는가) | ✅ 완전 보호 |
| 송신자 식별 (누가 보냈는가) | ⚠️ 송신자 주소 온체인 노출 |
| 지출 프라이버시 (어디로 나가는가) | ⚠️ 지출 트랜잭션 기록 남음 |

### 지출 프라이버시 강화 방안 (다음 연구 과제)

- **Relayer (메타트랜잭션)**: Alice가 서명만 하고 릴레이어가 대신 브로드캐스트하여 지출 개시자 신원 보호
- **ZK Proof**: Tornado Cash 방식으로 스텔스 주소와 수신자 주소 간 연결을 수학적으로 차단
- **스텔스 체이닝**: 스텔스 주소 → 새 스텔스 주소로 이어 보내 추적 난이도 증가

> ERC-5564는 **수신 프라이버시**를 표준화한 것입니다.  
> 완전한 트랜잭션 프라이버시는 ZK 레이어와의 결합으로 완성됩니다.

---

## 참고

- [ERC-5564: Stealth Addresses](https://eips.ethereum.org/EIPS/eip-5564)
- [ERC-6538: Stealth Meta-Address Registry](https://eips.ethereum.org/EIPS/eip-6538)
- [beatoz](https://beatoz.io)
