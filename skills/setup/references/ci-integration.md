# CI への組み込み

CI で回すコマンド。ツールを用意する方法はプロジェクトに任せる。go.mod の tool directive で入れたツールは
`go tool analyze-modularity --strict .` のように `go tool` を前に付けて呼ぶ。
golangci-lint は v2 系を使う (v1 と設定非互換)。

```sh
golangci-lint run --timeout 5m ./...
go-arch-lint check
analyze-arch-lint --strict .
vulncheck ./...
gosec ./...
analyze-modularity --strict .
go test -coverprofile=coverage.out ./...
```

`analyze-modularity --strict` がどの警告で exit 1 にするかは、
`go-arch-metrics:evaluate` の「`--strict` で exit 1 になる警告」。

揮発性は git の履歴から測るので、`analyze-modularity` を回す job は履歴をすべて取得する。
付けないと shallow clone になり、揮発性の判定は効かない (すべて揮発として扱い、stderr に案内が出る)。

```yaml
- uses: actions/checkout@v6
  with:
    fetch-depth: 0
```

### base と比べて判定する (`--baseline`)

揮発の窓は判定する変更自身の commit も含むので、よく触るパッケージは中身によらず落ちうる。
base の commit で測ったレポートを `--baseline` に渡すと、base から D を悪くしたパッケージだけを落とす。
判定の規則は `go-arch-metrics:evaluate` の `references/thresholds.md`「base との比較」。

守る決まりは「base と今の 2 回を、今の checkout から解決した同じバイナリで測る」こと。
`go tool` で呼ぶなら、今の checkout を cwd にして base の worktree を引数に渡せば、版は今の `go.mod` のものになる。
`go.mod` で版を上げた PR も、base を新しい版で測るので測り方の違いでは落ちない。
nix や aqua でツールを入れているなら、同じ決まりを守る形に読み替える。base 側は `--strict` を付けない。

下の例は `pull_request` と `push` の job 向け。`pull_request` は `actions/checkout` の既定 (base の先頭へ PR を
merge した ref) を前提に、`base.sha` と比べる。`pull_request.head.sha` を checkout する設定なら、比べる先は
`git merge-base HEAD origin/$GITHUB_BASE_REF` にする (base の先頭と比べると、base 側の変更まで PR のせいになる)。
新しいブランチの最初の push (`before` がすべて 0)、`workflow_dispatch`、`schedule` では比べる base が無いので、
`--baseline` なしの判定になる。

```yaml
- name: Measure base
  id: base
  env:
    # PR は base ブランチの先頭、push は push 前の先頭と比べる
    BASE_SHA: ${{ github.event.pull_request.base.sha || github.event.before }}
  if: env.BASE_SHA != '' && !startsWith(env.BASE_SHA, '0000000')
  run: |
    git worktree add --detach "$RUNNER_TEMP/base" "$BASE_SHA"
    go tool analyze-modularity "$RUNNER_TEMP/base" > "$RUNNER_TEMP/base.json"
    echo "args=--baseline $RUNNER_TEMP/base.json" >> "$GITHUB_OUTPUT"
- run: go tool analyze-modularity --strict ${{ steps.base.outputs.args }} .
```

base の commit を checkout するので、この job も `fetch-depth: 0` が要る。

ベースライン測定 (`arch-baseline`) は上のコマンドを一通り回してサマリと JSON を出すもので、
CI のゲートには使わない。ゲートは各ツールの終了コードで個別に掛ける。
数値を見たいときは `go-arch-metrics:measure` から実行する。

---

## PR ゲートとしての活用

### 段階的導入

golangci-lint v2 では `issues.new` / `issues.new-from-rev` の設定キーが廃止された。
既存違反が多いプロジェクトでの段階的導入は、**しきい値を実測最大値の直上に置いて
CI を緑にし、そこから絞る** 方式で行う (`references/golangci-config.md` の
「しきい値の調整」)。

### 違反数レポートをコメントとして投稿

```yaml
- name: Run golangci-lint with JSON output
  run: |
    golangci-lint run --output.json.path stdout ./... 2>/dev/null > lint-result.json || true

- name: Post lint summary
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const result = JSON.parse(fs.readFileSync('lint-result.json', 'utf8'));
      const issues = result.Issues || [];
      const summary = issues.reduce((acc, issue) => {
        acc[issue.FromLinter] = (acc[issue.FromLinter] || 0) + 1;
        return acc;
      }, {});
      const body = Object.entries(summary)
        .map(([linter, count]) => `- ${linter}: ${count} violations`)
        .join('\n');
      github.rest.issues.createComment({
        issue_number: context.issue.number,
        owner: context.repo.owner,
        repo: context.repo.repo,
        body: `## Architecture Metrics\n\n${body || 'No violations found!'}`
      });
```
