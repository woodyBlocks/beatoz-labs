# beatoz-labs/stealth-account Makefile
# ─────────────────────────────────────────────────────────────────────────────
# 사용법:
#   make build-contracts   - Solidity 컴파일 및 ABI 파일 생성
#   make tidy              - Go 의존성 정리
#   make test-crypto       - 크립토 단위 테스트 (노드 불필요)
#   make test              - 통합 테스트 (실행 중인 beatoz 노드 필요)
#   make funder            - 테스트용 펀더 지갑 생성
#   make clean             - 생성 파일 삭제

.PHONY: build-contracts tidy test-crypto test funder clean

FORGE     := forge
JQ        := jq
GO        := go
CONTRACTS := ./contracts
ABIS      := ./abis

# ─── 컨트랙트 빌드 ───────────────────────────────────────────────────────────
# Foundry로 컴파일 후, beatoz vm.NewEVMContract가 읽는 형식으로 변환한다.
# beatoz ABI 형식: {"contractName": "...", "abi": [...], "bytecode": "0x...", "deployedBytecode": "0x..."}
# Foundry 출력 형식: {"abi": [...], "bytecode": {"object": "0x..."}, ...}

build-contracts:
	@echo "=== Solidity 컴파일 ==="
	cd $(CONTRACTS) && $(FORGE) build
	@echo "=== ABI 파일 변환 (Foundry → beatoz 형식) ==="
	@mkdir -p $(ABIS)
	$(JQ) '{contractName: "ERC5564Announcer", abi: .abi, bytecode: .bytecode.object, deployedBytecode: .deployedBytecode.object}' \
		$(CONTRACTS)/out/ERC5564Announcer.sol/ERC5564Announcer.json > $(ABIS)/ERC5564Announcer.json
	$(JQ) '{contractName: "ERC6538Registry", abi: .abi, bytecode: .bytecode.object, deployedBytecode: .deployedBytecode.object}' \
		$(CONTRACTS)/out/ERC6538Registry.sol/ERC6538Registry.json > $(ABIS)/ERC6538Registry.json
	@echo "생성된 ABI 파일:"
	@ls -la $(ABIS)/

# ─── Go 의존성 ───────────────────────────────────────────────────────────────
tidy:
	$(GO) mod tidy

# ─── 테스트 ──────────────────────────────────────────────────────────────────
# 크립토 단위 테스트 — beatoz 노드 불필요
test-crypto:
	@echo "=== 크립토 단위 테스트 ==="
	$(GO) test -v -run TestStealthCrypto ./...

# 통합 테스트 — 실행 중인 beatoz 노드 필요
# 환경 변수:
#   BEATOZ_RPC_URL     (기본: http://localhost:36657)
#   BEATOZ_WS_URL      (기본: ws://localhost:36657/websocket)
#   BEATOZ_FUNDER_KEY  (기본: ./testdata/funder.json)
#   BEATOZ_FUNDER_PASS (기본: 1111)
test: build-contracts
	@echo "=== 통합 테스트 ==="
	$(GO) test -v -run TestStealthAccount -timeout 120s ./...

# 모든 테스트
test-all: build-contracts
	$(GO) test -v -timeout 120s ./...

# ─── 펀더 지갑 생성 ──────────────────────────────────────────────────────────
# 이미 지갑 파일이 있으면 건너뛴다.
funder:
	@mkdir -p testdata
	@if [ -f testdata/funder.json ]; then \
		echo "펀더 지갑이 이미 존재합니다: testdata/funder.json"; \
	else \
		echo "펀더 지갑 생성 중..."; \
		$(GO) run ./cmd/mkwallet -out testdata/funder.json -pass 1111; \
		echo "생성 완료: testdata/funder.json"; \
		echo "이 지갑에 beatoz 노드에서 충분한 BTOS를 전송하세요."; \
	fi

# ─── 정리 ────────────────────────────────────────────────────────────────────
clean:
	rm -rf $(CONTRACTS)/out $(CONTRACTS)/cache
	rm -f $(ABIS)/*.json
	@echo "정리 완료"
