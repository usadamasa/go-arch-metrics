// vulncheck は govulncheck をライブラリとして組み込んだ CLI｡引数と exit code は govulncheck と同じ
// (0 = 脆弱性なし、3 = 脆弱性あり、それ以外 = 実行できていない)｡
//
// 素の govulncheck と違うのは TLS の証明書検証だけ｡OS の証明書ストアに頼らず、埋め込みの fallback roots
// (Mozilla の bundle) で検証する｡macOS の sandbox (Seatbelt) の中では trustd に届かず、crypto/x509 が
// `x509: OSStatus -26276` で落ちるため｡下の go:debug で全プラットフォームで常時有効にしている｡
// 社内 CA で TLS を中継する環境では GODEBUG=x509usefallbackroots=0 で OS の検証に戻せる｡
//
//go:debug x509usefallbackroots=1
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	_ "golang.org/x/crypto/x509roots/fallback"
	"golang.org/x/vuln/scan"
)

func main() {
	cmd := scan.Command(context.Background(), os.Args[1:]...)
	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	if err == nil {
		return
	}

	// govulncheck 本体 (cmd/govulncheck/main.go) と同じ対応: 脆弱性ありは ExitCode を持つ error で返る｡
	// 他の error で包まれていないときは govulncheck が既に stderr に出しているので、二重に出さない
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		if _, bare := err.(interface{ ExitCode() int }); !bare {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
