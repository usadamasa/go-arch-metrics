# CI への組み込み

CI で回すコマンド。ツールを PATH に用意する方法はプロジェクトに任せる。
golangci-lint は v2 系を使う (v1 と設定非互換)。

```sh
golangci-lint run --timeout 5m ./...
go-arch-lint check
analyze-arch-lint --strict .
govulncheck ./...
gosec ./...
analyze-modularity --strict .
spm-go all
go test -coverprofile=coverage.out ./...
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
