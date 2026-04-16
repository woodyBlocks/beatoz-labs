// cmd/mkwallet은 테스트용 펀더 지갑을 생성하고 파일에 저장한다.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beatoz/beatoz-go/libs"
	btzweb3 "github.com/beatoz/beatoz-sdk-go/web3"
)

func main() {
	outPath := flag.String("out", "testdata/funder.json", "지갑 파일 저장 경로")
	pass := flag.String("pass", "1111", "지갑 비밀번호")
	flag.Parse()

	w := btzweb3.NewWallet([]byte(*pass))

	f := libs.NewFileWriter(*outPath)
	if err := w.Save(f); err != nil {
		fmt.Fprintf(os.Stderr, "지갑 저장 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("지갑 생성 완료: %s\n", *outPath)
	fmt.Printf("주소: %X\n", w.Address())
}
