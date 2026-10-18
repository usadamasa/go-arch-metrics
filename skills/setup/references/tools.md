# ツール詳細リファレンス

出力例は `go-arch-metrics:measure` の `references/output-examples.md`、
しきい値の根拠は `go-arch-metrics:evaluate` の `references/thresholds.md` にある。

---

## この plugin の CLI を入れる

`analyze-modularity`・`analyze-arch-lint`・`vulncheck` は、この plugin のリポジトリの flake が
`packages.<system>.default` として出す (x86_64-linux、aarch64-linux、aarch64-darwin)。
nix を使うプロジェクトは flake input で参照し、devShell に足す。

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # tag は付け替えられるので、固定は flake.lock の commit に任せる
    go-arch-metrics.url = "github:usadamasa/go-arch-metrics";
  };

  outputs = { nixpkgs, go-arch-metrics, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      devShells.${system}.default = pkgs.mkShellNoCC {
        packages = [
          pkgs.go
          pkgs.git
          pkgs.go-arch-lint
          go-arch-metrics.packages.${system}.default
        ];
      };
    };
}
```

ビルドとテストは本体のリポジトリの CI が持つ。利用側で `buildGoModule` を書いて自前でビルドしない。

3 本とも実行時に PATH のコマンドを呼ぶ。package はこれらを包まないので、利用側の devShell に並べる。
`go` を包まないのは、対象プロジェクトの `go.mod` に合った版で `go list` を走らせるため。

| CLI | 実行時に要るコマンド |
|-----|------------------|
| `analyze-modularity` | `go`、`git` |
| `analyze-arch-lint` | `go`、`go-arch-lint` |
| `vulncheck` | `go` |

---

## golangci-lint

### 概要

複数の linter をまとめて実行する Go 静的解析ツール。
アーキテクチャメトリクス向けには以下の linter を有効化する:

| Linter | 計測するメトリクス |
|--------|------------------|
| `gocognit` | 認知的複雑度 |
| `gocyclo` | 循環複雑度 (McCabe) |
| `cyclop` | 循環複雑度 (パッケージ全体も対象) |
| `funlen` | 関数の行数・文数 |
| `nestif` | ネストの深さ |
| `maintidx` | 保守性指数 |
| `staticcheck` | 高度なバグ検出・未使用コード (U1000 系) |
| `depguard` | import 禁止リスト |

しきい値と根拠は `go-arch-metrics:evaluate` の `references/thresholds.md`。

> **注意**: `deadcode` は golangci-lint v2 に存在しない。`golangci-lint run --enable deadcode` を実行すると `Error: unknown linters: 'deadcode'` になる。未使用コードの検出は `staticcheck` の U1000 系チェックが代替する。

### 実行

```bash
# 実行
golangci-lint run ./...

# 特定 linter のみ
golangci-lint run --enable-only gocognit,gocyclo ./...

# JSON 出力 (v2)
golangci-lint run --output.json.path stdout ./... 2>/dev/null
```

---

## go-arch-lint

### 概要

パッケージ間の依存方向を `.go-arch-lint.yml` で宣言し、違反を検出するツール。
レイヤードアーキテクチャ・クリーンアーキテクチャの依存方向を強制できる。

### 実行

```bash
# 実行 (プロジェクトルートから)
go-arch-lint check

# JSON 出力
go-arch-lint check --json

# 依存関係のグラフ生成 (./go-arch-lint-graph.svg に出る)
go-arch-lint graph
```

---

## vulncheck (govulncheck の組み込み)

### 概要

Go の既知の脆弱性データベース (vuln.go.dev) に基づき、呼んでいるシンボルに脆弱性がないかスキャンする
govulncheck (Go チーム公式) を、この plugin がライブラリとして組み込んだ CLI。引数と exit code は
govulncheck と同じ (0 = 脆弱性なし、3 = 脆弱性あり、それ以外 = 実行できていない)。

違いは TLS の証明書検証だけ。OS の証明書ストアに頼らず、埋め込みの roots (Mozilla の bundle) で検証する。
macOS の sandbox (Seatbelt) の中では trustd に届かず、素の govulncheck は `x509: OSStatus -26276` で落ちるため。
社内 CA で TLS を中継する環境では `GODEBUG=x509usefallbackroots=0` を付けると OS の検証に戻る。

### 実行

```bash
# 実行
vulncheck ./...

# ローカルの DB を使う (ネットワークが無い環境、テスト)
vulncheck -db file:///path/to/vulndb ./...
```

---

## gosec

### 概要

Go ソースコードのセキュリティ問題を検出する静的解析ツール。
SQL インジェクション、ハードコードされた認証情報、弱い暗号等を検出する。

### 実行

```bash
# 実行
gosec ./...
```

---

## analyze-modularity

### 概要

パッケージの API surface area と、Robert C. Martin の Package Metrics (Ca/Ce/A/I/D) を測るカスタム CLI。
前者は `go/ast` で宣言を数え、後者は `go list -json ./...` の import 辺から出す。

| メトリクス | 説明 |
|-----------|------|
| Public ratio | exported / (exported + unexported) |
| LOC 外れ値 | パッケージ LOC > mean + 2σ |
| Exported funcs/methods | 公開関数・メソッド数 |
| Exported types | 公開型 (struct/interface) 数 |
| Afferent Coupling (Ca) | このパッケージに依存するモジュール内パッケージ数 |
| Efferent Coupling (Ce) | このパッケージが依存するモジュール内パッケージ数 (標準ライブラリ・外部モジュール・テストファイルの import は数えない) |
| Abstractness (A) | interface 型宣言 / 全型宣言 |
| Instability (I) | Ce / (Ca + Ce)。孤立パッケージ (Ca = Ce = 0) は 1 |
| Distance (D) | \|A + I - 1\| (Main Sequence からの距離) |

Zone 分類と構造的制約の扱いは `go-arch-metrics:evaluate`。

対象は `go list ./...` が列挙するパッケージ (入れ子のモジュールと vendor を含まない) のうち、
git が ignore していないもの。測るのは構成管理の対象だけで、`tmp/` のような ignore 済みの試し書きは
一覧にも Ca/Ce の辺にも入らない。前提は `go.mod` のあるモジュールで `go list ./...` が通ること。
git リポジトリでなければ ignore するものは無いとみなす (stderr に 1 行出る)。

### 実行

```bash
# 実行
analyze-modularity <directory>...

# フラグ
analyze-modularity --max-public-ratio 0.6 --sigma 2.0 --min-symbols 10 .

# JSON 出力を jq で整形
analyze-modularity . | jq '.summary'
analyze-modularity . | jq '.warnings'
```

---

## analyze-arch-lint

### 概要

`.go-arch-lint.yml` 自体の健全性を測るカスタム CLI。go-arch-lint check が見るのは
「コードが設定に違反していないか」だけで、設定がパッケージツリーの転記へ劣化していく
方向は検出できない。そこを数値にする。観点は `go-arch-metrics:evaluate`。

**指標の一覧・しきい値・意味・対処は `analyze-arch-lint --metrics` が出す。**
ここにもドキュメント側にも表を置かない。

### 実行

```bash
# 実行
analyze-arch-lint <project-dir>

# 指標の一覧・しきい値・意味・対処
analyze-arch-lint --metrics

# CI ゲート (しきい値超過で exit 1、違反ごとに意味と対処を stderr へ)
analyze-arch-lint --strict <project-dir>

# JSON 出力
analyze-arch-lint --json <project-dir> | jq '.findings'
```

`go-arch-lint mapping --json` と `go list` を呼ぶので、どちらも PATH に要る。
