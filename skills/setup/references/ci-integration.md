# CI への組み込み

CI で回すコマンド。ツールを PATH に用意する方法はプロジェクトに任せる。
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

`analyze-modularity --strict` が exit 1 にするのは、コードを直して消せる警告 (`actionable: true`) があるときだけ。
構造制約のパッケージ (Ce=0, Ca>0) と非揮発の Zone of Pain に付く警告は JSON に出るが、
失敗には数えない。判断の基準は `go-arch-metrics:evaluate`。

揮発性は git の履歴から測るので、`analyze-modularity` を回す job は履歴をすべて取得する。
付けないと shallow clone になり、揮発性の判定は効かない (すべて揮発として扱い、stderr に案内が出る)。

```yaml
- uses: actions/checkout@v6
  with:
    fetch-depth: 0
```

ベースライン測定 (`baseline.sh`) はプラグインのディレクトリに置かれ、パスが
プラグインのバージョンによって変わるため CI には書かない。
`go-arch-metrics:measure` から実行する。

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
