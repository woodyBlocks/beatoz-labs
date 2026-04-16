package stealthlab_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	ctrlertypes "github.com/beatoz/beatoz-go/ctrlers/types"
	btztypes "github.com/beatoz/beatoz-go/types"
	"github.com/beatoz/beatoz-go/types/xerrors"
	btzweb3 "github.com/beatoz/beatoz-sdk-go/web3"
	"github.com/holiman/uint256"
	tmjson "github.com/tendermint/tendermint/libs/json"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

// ─── 환경 설정 ─────────────────────────────────────────────────────────────────
// 테스트 실행 전 다음 환경 변수를 설정해야 한다:
//
//   BEATOZ_RPC_URL      - beatoz 노드 HTTP RPC (기본: http://localhost:36657)
//   BEATOZ_WS_URL       - beatoz 노드 WebSocket (기본: ws://localhost:36657/websocket)
//   BEATOZ_FUNDER_KEY   - 펀딩용 지갑 파일 경로 (기본: ./testdata/funder.json)
//   BEATOZ_FUNDER_PASS  - 펀딩용 지갑 비밀번호 (기본: 1111)

var (
	rpcURL    string
	wsURL     string
	bzweb3    *btzweb3.BeatozWeb3
	funder    *btzweb3.Wallet
	govParams *ctrlertypes.GovParams

	setupOnce sync.Once
	setupErr  error
)

func setup(t *testing.T) {
	t.Helper()
	setupOnce.Do(func() {
		rpcURL = envOrDefault("BEATOZ_RPC_URL", "http://localhost:36657")
		wsURL = envOrDefault("BEATOZ_WS_URL", "ws://localhost:36657/websocket")
		walletPath := envOrDefault("BEATOZ_FUNDER_KEY", "./testdata/funder.json")
		walletPass := []byte(envOrDefault("BEATOZ_FUNDER_PASS", "1111"))

		// NewBeatozWeb3는 Genesis()를 호출하므로 노드 연결 실패 시 panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					setupErr = fmt.Errorf("beatoz 노드 연결 실패 (%s): %v\n→ BEATOZ_RPC_URL을 확인하세요", rpcURL, r)
				}
			}()
			bzweb3 = btzweb3.NewBeatozWeb3(btzweb3.NewHttpProvider(rpcURL))
		}()
		if setupErr != nil {
			return
		}

		fmt.Printf("[setup] 노드 연결 성공: %s (chainID=%s)\n", rpcURL, bzweb3.ChainID())

		// 거버넌스 파라미터 로드 (가스 설정에 사용)
		var err error
		govParams, err = bzweb3.GetGovParams()
		if err != nil {
			setupErr = fmt.Errorf("거버넌스 파라미터 조회 실패: %w", err)
			return
		}

		// 펀더 지갑 로드
		f, err := os.Open(walletPath)
		if err != nil {
			setupErr = fmt.Errorf("펀더 지갑 파일 열기 실패 (%s): %w\n→ make funder 또는 BEATOZ_FUNDER_KEY를 설정하세요", walletPath, err)
			return
		}
		defer f.Close()

		funder, err = btzweb3.OpenWallet(f)
		if err != nil {
			setupErr = fmt.Errorf("펀더 지갑 열기 실패: %w", err)
			return
		}
		if err = funder.Unlock(walletPass); err != nil {
			setupErr = fmt.Errorf("펀더 지갑 언락 실패: %w", err)
			return
		}
		if err = funder.SyncAccount(bzweb3); err != nil {
			setupErr = fmt.Errorf("펀더 계정 동기화 실패: %w", err)
			return
		}
		fmt.Printf("[setup] 펀더: %X (잔액: %s)\n", funder.Address(), funder.GetBalance().Dec())
	})

	if setupErr != nil {
		t.Skip("테스트 환경 설정 실패:", setupErr)
	}
}

// ─── 공통 헬퍼 ──────────────────────────────────────────────────────────────────

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// newFundedWallet은 새 지갑을 생성하고 funder로부터 amount만큼 펀딩한다.
func newFundedWallet(t *testing.T, name string, amount *uint256.Int) *btzweb3.Wallet {
	t.Helper()
	pass := []byte("test1111")
	w := btzweb3.NewWallet(pass)
	require_no_error(t, w.Unlock(pass))

	if amount.Sign() > 0 {
		require_no_error(t, funder.SyncAccount(bzweb3))
		ret, err := funder.TransferCommit(w.Address(), defGas(), defGasPrice(), amount, bzweb3)
		require_no_error(t, err)
		require_success(t, ret.CheckTx.Code, ret.CheckTx.Log)
		require_success(t, ret.DeliverTx.Code, ret.DeliverTx.Log)
		funder.AddNonce()

		require_no_error(t, w.SyncAccount(bzweb3))
		fmt.Printf("[setup] %s 지갑: %X (잔액: %s)\n", name, w.Address(), w.GetBalance().Dec())
	}
	return w
}

// waitTrx는 트랜잭션이 블록에 포함될 때까지 최대 30초 대기한다.
func waitTrx(t *testing.T, txHash []byte) *coretypes.ResultTx {
	t.Helper()
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		res, err := bzweb3.GetTransaction(txHash)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				continue
			}
			t.Fatalf("트랜잭션 조회 실패: %v", err)
		}
		return res.ResultTx
	}
	t.Fatal("트랜잭션 타임아웃 (30초)")
	return nil
}

// waitNewBlock은 새 블록이 생성될 때까지 WebSocket으로 대기한다.
func waitNewBlock(t *testing.T) int64 {
	t.Helper()
	var height int64
	wg := sync.WaitGroup{}
	sub, err := btzweb3.NewSubscriber(wsURL)
	require_no_error(t, err)
	defer sub.Stop()

	wg.Add(1)
	err = sub.Start("tm.event='NewBlock'", func(_ *btzweb3.Subscriber, raw []byte) {
		evt := &coretypes.ResultEvent{}
		if e := tmjson.Unmarshal(raw, evt); e == nil {
			wg.Done()
			_ = evt
		}
	})
	require_no_error(t, err)
	wg.Wait()
	return height
}

// ─── 가스 설정 ─────────────────────────────────────────────────────────────────

const contractGasLimit = int64(3_000_000)

func defGasPrice() *uint256.Int {
	if govParams != nil {
		return govParams.GasPrice()
	}
	return uint256.NewInt(100)
}

func defGas() int64 {
	if govParams != nil {
		return govParams.MinTrxGas()
	}
	return int64(21_000)
}

// ─── 어서션 헬퍼 ────────────────────────────────────────────────────────────────

func require_no_error(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("예상치 못한 오류: %v", err)
	}
}

func require_success(t *testing.T, code uint32, log string) {
	t.Helper()
	if code != xerrors.ErrCodeSuccess {
		t.Fatalf("트랜잭션 실패 (code=%d): %s", code, log)
	}
}

func require_equal(t *testing.T, expected, actual interface{}, msg string) {
	t.Helper()
	e := fmt.Sprintf("%v", expected)
	a := fmt.Sprintf("%v", actual)
	if e != a {
		t.Fatalf("%s\n  expected: %v\n  actual:   %v", msg, expected, actual)
	}
}

func require_not_nil(t *testing.T, v interface{}, msg string) {
	t.Helper()
	if v == nil {
		t.Fatalf("%s: nil이 아니어야 함", msg)
	}
}

func require_true(t *testing.T, cond bool, msg string) {
	t.Helper()
	if !cond {
		t.Fatalf("%s: true이어야 함", msg)
	}
}

// ─── 디버그 유틸 ────────────────────────────────────────────────────────────────

func prettyJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// addrFromBytes는 20바이트 슬라이스를 btztypes.Address로 변환한다.
func addrFromBytes(b []byte) btztypes.Address {
	addr := make(btztypes.Address, 20)
	copy(addr, b)
	return addr
}
