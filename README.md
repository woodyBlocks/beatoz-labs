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
| 실행 일시 | `2026-04-16` |
| 실행 명령 | `go test -run TestStealthAccount -v ./...` |

### 실행 결과

```
--- PASS: TestStealthAccount (12.10s)
```

### 실제 주소

| 역할 | 주소 |
|------|------|
| Funder | `7EFE0FCD1A2074F6523D4BE591DD45DB8468373E` |
| Deployer | `403BB8C1E392F49C84A5A1C4CBD4C6E7A96D1E18` |
| Bob (송신자) | `D8B75B98DB680A673D3966C0302FD4B63733818F` |
| Alice (수신자) | `19D43DC5FB6C1D0E48EFC8096B875E49D9AFA0B2` |
| ERC5564Announcer | `E2CD2BB64FE34BF926A3B007128C5214D65BC9EA` |
| ERC6538Registry | `1E45C69E43FFED39C56CAB0679360760E3AACD08` |
| 스텔스 주소 | `AF37FC29769F4B36B843A3FC7666D5CC0F4409EB` |
| 지출 대상 임시 주소 | `F235E58478549EAD71C16572AF99840CD6DAEEA4` |

### 스텔스 파라미터

| 항목 | 값 |
|------|----|
| Alice 메타-주소 | `02C524F69162E806DC3A9C84BEC51F715DEB1B044B8791EF1A6711B91B82FBFE2A03F5811D4DC9825A1F57C872AD9A2D6B5F0756E99E92C0A94C6605CBA35FE5D80C` |
| Ephemeral PubKey | `02CC4BE6749EA77930A27FA3F766AF2BAB9DF17172D6F4E77966AB30D5EC1E951F` |
| View Tag | `0xF1` |
| schemeId | `1` |

### 실제 트랜잭션

| Height | TxHash | From | To | 내용 |
|-------:|--------|------|----|------|
| `25188` | `1294E5E911FD13401EF7BD46EB73BC892FC0AB3869492B7873F4D2CAD0ECEBBB` | `7EFE0FCD1A2074F6523D4BE591DD45DB8468373E` | `403BB8C1E392F49C84A5A1C4CBD4C6E7A96D1E18` | deployer 지갑에 `5 BTZ` 펀딩 |
| `25189` | `DDB3820B7495A2AA7FB9B1C1BB95D67C993907E41701B0F8D50CD64508D56707` | `7EFE0FCD1A2074F6523D4BE591DD45DB8468373E` | `D8B75B98DB680A673D3966C0302FD4B63733818F` | Bob 지갑에 `5 BTZ` 펀딩 |
| `25190` | `E6E3CFA3F2F14564CDEDD86F2C3E531F8D58EE16FC6314896E923C28F49AC980` | `7EFE0FCD1A2074F6523D4BE591DD45DB8468373E` | `19D43DC5FB6C1D0E48EFC8096B875E49D9AFA0B2` | Alice 지갑에 `5 BTZ` 펀딩 |
| `25191` | `FDC97F06CF42084E49F8DDC00245A6516CB4756673FACFE36D5A9F108B89402F` | `403BB8C1E392F49C84A5A1C4CBD4C6E7A96D1E18` | `0x0000000000000000000000000000000000000000` | `ERC5564Announcer` 배포, 생성 주소 `E2CD2BB64FE34BF926A3B007128C5214D65BC9EA` |
| `25192` | `592C27926FB5B78701E117755343F3B88B16A312F78143299836324C9CD7417F` | `403BB8C1E392F49C84A5A1C4CBD4C6E7A96D1E18` | `0x0000000000000000000000000000000000000000` | `ERC6538Registry` 배포, 생성 주소 `1E45C69E43FFED39C56CAB0679360760E3AACD08` |
| `25193` | `AA1FD380F82745E0EA936DFF2DB7C802B893195EAF58786DCE5D5BFD4205B9DA` | `19D43DC5FB6C1D0E48EFC8096B875E49D9AFA0B2` | `1E45C69E43FFED39C56CAB0679360760E3AACD08` | `registerKeys(1, metaAddress)` 호출로 Alice 메타-주소 등록 |
| `25194` | `D3E38138D7CB780FB838834218636F0FAF8DECCCA4AEB23DDDAB9A4F706A5589` | `D8B75B98DB680A673D3966C0302FD4B63733818F` | `AF37FC29769F4B36B843A3FC7666D5CC0F4409EB` | Bob이 스텔스 주소에 `2 BTZ` 전송 |
| `25195` | `A8FD7EBB9B5957C88A84F38930B3C33F449D896CB9230DC709635333B8825029` | `D8B75B98DB680A673D3966C0302FD4B63733818F` | `E2CD2BB64FE34BF926A3B007128C5214D65BC9EA` | `announce(1, stealthAddress, ephemeralPubKey, 0x01f1)` 호출 |
| `25196` | `50D3070C915A17308946B9C341AA568F77A5F5D3D01C3A34F03F3CFCAB53CEB8` | `AF37FC29769F4B36B843A3FC7666D5CC0F4409EB` | `F235E58478549EAD71C16572AF99840CD6DAEEA4` | Alice가 스텔스 개인키로 `1 BTZ` 지출 |

### 컨트랙트 호출 내용

| 트랜잭션 | 호출 대상 | 파라미터 |
|---------|-----------|----------|
| `AA1FD380F82745E0EA936DFF2DB7C802B893195EAF58786DCE5D5BFD4205B9DA` | `ERC6538Registry.registerKeys` | `schemeId = 1`, `stealthMetaAddress = 0x02C524...5D80C` |
| `A8FD7EBB9B5957C88A84F38930B3C33F449D896CB9230DC709635333B8825029` | `ERC5564Announcer.announce` | `schemeId = 1`, `stealthAddress = AF37FC29769F4B36B843A3FC7666D5CC0F4409EB`, `ephemeralPubKey = 0x02CC4B...951F`, `metadata = 0x01F1` |

### 단계별 검증

| 단계 | 내용 | 결과 |
|------|------|------|
| Step 1 | ERC5564Announcer + ERC6538Registry 컨트랙트 배포 | ✅ |
| Step 2 | Alice 메타-주소 레지스트리 등록 | ✅ |
| Step 3 | Bob이 스텔스 주소 파생 (뷰 태그 `0xF1`) | ✅ |
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
