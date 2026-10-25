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
    # package は nixpkgs の go_latest でビルドされる。vulncheck は自分をビルドした Go より新しい
    # go 行のモジュールを測れないので、利用側の nixpkgs に揃えて利用側の go に追いつかせる
    go-arch-metrics.inputs.nixpkgs.follows = "nixpkgs";
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
入れた直後の flake.lock は default branch の HEAD を指すので、下の「版を plugin にそろえる」で tag の commit に合わせる。

3 本とも実行時に PATH のコマンドを呼ぶ。package はこれらを包まないので、利用側の devShell に並べる。
`go` を包まないのは、対象プロジェクトの `go.mod` に合った版で `go list` を走らせるため。

| CLI | 実行時に要るコマンド |
|-----|------------------|
| `analyze-modularity` | `go`、`git` |
| `analyze-arch-lint` | `go`、`go-arch-lint` |
| `vulncheck` | `go` |

### nix を使わないプロジェクト (go.mod の tool directive)

go.mod の tool directive に足し、`go tool <name>` で呼ぶ。`baseline.sh` は PATH に無いツールを
tool directive から探すので、PATH に載せなくてよい。

```bash
# VERSION は plugin の版 (下の「版を plugin にそろえる」)
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity@"$VERSION"
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint@"$VERSION"
go get -tool github.com/usadamasa/go-arch-metrics/cmd/vulncheck@"$VERSION"
```

この plugin の tag は Go の semver tag ではない (`v` を付けない CalVer)。tag 名を指定すると、その tag の commit の
pseudo-version (`v0.0.0-<日時>-<commit の先頭 12 桁>`) に解決され、go.mod の require はその commit に固定される。
CI のコマンドは `go tool vulncheck ./...` のように `go tool` を前に付けて書く。

- 対象プロジェクトの go.mod の `go` 行は 1.26.1 以上が要る (この plugin のモジュールの要求)。
  `golang.org/x/vuln` などの依存も indirect の require として入る
- 3 本は対象プロジェクトの go でビルドされる (この plugin の go.mod の `toolchain` 行は依存側では効かない)。
  crypto/x509 の GO-2026-6809 が直った go1.26.10 / go1.27.3 以上で動かす
- `go-arch-lint` は PATH に置く。`analyze-arch-lint` が PATH から呼ぶので、tool directive に入れると
  `baseline.sh` は動いても `analyze-arch-lint` が見つけられない

### 版を plugin にそろえる

plugin と CLI 3 本は、同じ release の tag の commit にそろえる。tag 名は plugin の版
(`claude plugin list` の `go-arch-metrics@usadamasa` の `version`) と同じ。取った後の照合は、上げるときと、初めて入れた直後
(Go は上の `go get -tool`、nix は flake input を足した直後) の両方で行う。

`go get ...@latest` や `nix flake update` のように「最新」を問い合わせる取り方はしない。理由は 2 つある。

- どちらも default branch の HEAD に解決される。tag は Go の semver tag ではないので `@latest` の候補にならず、
  release の後に入った commit があれば tag の commit からずれる
- 問い合わせの結果がキャッシュされ、release の直後には前の commit に解決されることがある
  - Go の module proxy (proxy.golang.org) は `@latest` の解決結果を proxy 側でキャッシュする。release の直後に
    `@latest` が前の release の commit (最新の tag より 23 commit 前) に解決された例がある
  - nix は default branch の HEAD を GitHub API で引いた結果を、手元で `tarball-ttl` (既定 3600 秒) の間使い回す

tag 名で問い合わせると、どちらもその時点の tag の commit に解決される。

```bash
VERSION=2026.1018.3   # plugin の版

# go.mod の tool directive に入れている (3 本は同じモジュールなので 1 回で全部上がる)
go get github.com/usadamasa/go-arch-metrics@"$VERSION"

# flake input で参照している (flake.nix の url はそのまま。flake.lock の locked だけが動く)
nix flake lock --override-input go-arch-metrics "github:usadamasa/go-arch-metrics/$VERSION"
```

取った後、tag の commit と一致するかを確かめる。tag は付け替えられ、proxy や nix のキャッシュは
付け替え前の commit を返し続けることがあるため。

```bash
# tag の commit (40 桁)。tagpr の tag は lightweight なので 1 行で返る。annotated なら `^{}` の付いた行が commit
git ls-remote --tags https://github.com/usadamasa/go-arch-metrics "$VERSION"

# go.mod: pseudo-version の末尾 12 桁が tag の commit の先頭 12 桁と一致するか
go list -m github.com/usadamasa/go-arch-metrics

# flake.lock: locked.rev が tag の commit と一致するか
jq -r '.nodes["go-arch-metrics"].locked.rev' flake.lock
```

ずれていたら、tag 名の代わりに `git ls-remote` が返した commit で取り直す。
nix は短縮した commit を受け付けないので 40 桁で渡す。

```bash
go get github.com/usadamasa/go-arch-metrics@<commit>
nix flake lock --override-input go-arch-metrics github:usadamasa/go-arch-metrics/<commit>
```

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
前者は `go/ast` で宣言を数え、`golang.org/x/tools/go/packages` の型情報で外からの参照を解く。
後者は同じ型情報で、宣言どうしのパッケージをまたぐ参照から出す。

| メトリクス | 説明 |
|-----------|------|
| 外から参照されない exported | unexport しても他の挙動が変わらない exported の数と名前。判定の規則は `go-arch-metrics:evaluate` の `references/thresholds.md` |
| Public ratio | exported / (exported + unexported)。参考値で `--strict` の判定には使わない |
| LOC 外れ値 | パッケージ LOC > mean + 2σ |
| Exported funcs/methods | 公開関数・メソッド数 |
| Exported types | 公開型 (struct/interface) 数 |
| Afferent Coupling (Ca) | このパッケージを参照する宣言を持つ、モジュール内のほかのパッケージの数 |
| Efferent Coupling (Ce) | このパッケージの宣言のうち、モジュール内のほかのパッケージを参照するものの数 (標準ライブラリ・外部モジュール・テストファイルは数えない) |
| Abstractness (A) | interface 型宣言 / 全型宣言 |
| Instability (I) | Ce / (Ca + Ce)。孤立パッケージ (Ca = Ce = 0) は 1 |
| Distance (D) | \|A + I - 1\| (Main Sequence からの距離) |

Ca と Ce は、どちらもそのパッケージのコードが変わらない限り動かないように数える単位を選んでいる。

- Ce は自分の宣言を数える (Martin の定義でクラスを数えるのに当たる)。宣言は型とそのメソッドで 1 つ、
  それ以外の関数は 1 つずつ、var・const・type は spec ごとに 1 つ。依存先のパッケージの数で数えると、
  依存先を統合しただけで、コードが同じ呼び手の Ce が減って I が下がる
- Ca は依存元のパッケージを数える (JDepend の定義)。依存元の宣言の数で数えると、既存の呼び手の
  パッケージに関数を 1 つ足しただけで、呼ばれる側の Ca が増えて I が下がる

Ca が増えるのは、新しいパッケージが依存し始めたときだけになる。2 つの単位が違うので、外を参照する宣言の多い
大きなパッケージほど I は高めに出る。

Zone 分類と構造的制約の扱いは `go-arch-metrics:evaluate`。

対象は `go list ./...` が列挙するパッケージ (入れ子のモジュールと vendor を含まない) のうち、
git が ignore していないもの。測るのは構成管理の対象だけで、`tmp/` のような ignore 済みの試し書きは
一覧にも Ca/Ce の参照にも入らない。前提は `go.mod` のあるモジュールで `go list ./...` が通ること。
git リポジトリでなければ ignore するものは無いとみなす (stderr に 1 行出る)。

### 実行

```bash
# 実行
analyze-modularity <directory>...

# フラグ (--max-public-ratio と --min-symbols は参考情報 high_public_ratio のしきい値)
analyze-modularity --max-public-ratio 0.6 --sigma 2.0 --min-symbols 10 .

# JSON 出力を jq で整形
analyze-modularity . | jq '.summary'
analyze-modularity . | jq '.warnings'

# CI ゲート (actionable な警告があれば exit 1)
analyze-modularity --strict .

# 揮発性の窓としきい値 (既定は直近 200 commit で 5 回以上触られたら揮発)
analyze-modularity --volatility-window 200 --volatile-commits 5 .
```

揮発性は git の履歴から測る。shallow clone では測らず、Zone of Pain をすべて揮発として扱う
(stderr に 1 行出る)。窓としきい値の根拠は `go-arch-metrics:evaluate`。

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
