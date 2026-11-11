#!/usr/bin/env bash
# golang.org/x/crypto/x509roots/fallback を最新の pseudo-version へ上げ、flake.nix の vendorHash を付け直す。
# tag の無い module で Dependabot が解決できないため、update-x509roots.yaml が代わりに回す。
# リポジトリのルートで、devShell の go と nix がある状態で呼ぶ。
# GITHUB_OUTPUT があれば changed=true|false を書く。
set -euo pipefail

readonly MODULE="golang.org/x/crypto/x509roots/fallback"
readonly FAKE_HASH="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

for tool in go nix git; do
  if ! command -v "$tool" >/dev/null; then
    printf 'ERROR: %s が見つかりません\n' "$tool" >&2
    exit 1
  fi
done
if [ ! -f flake.nix ] || [ ! -f go.mod ]; then
  printf 'ERROR: リポジトリのルートで実行してください (flake.nix と go.mod が要る)\n' >&2
  exit 1
fi

write_output() {
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf 'changed=%s\n' "$1" >>"$GITHUB_OUTPUT"
  fi
}

go get "${MODULE}@latest"
go mod tidy

if git diff --quiet -- go.mod go.sum; then
  printf '%s は最新です\n' "$MODULE"
  write_output false
  exit 0
fi

# vendorHash の行がちょうど 1 つでなければ、どこを書き換えるか決まらない
# grep -c は 0 件のとき exit 1 になるので、件数の比較で判定する
if ! count=$(grep -c '^ *vendorHash = "sha256-[A-Za-z0-9+/=]*";' flake.nix); then
  count=0
fi
if [ "$count" != "1" ]; then
  printf 'ERROR: flake.nix の vendorHash の行が 1 つではありません (%s 行)\n' "$count" >&2
  exit 1
fi

set_vendor_hash() {
  local -r hash="$1"
  sed -i -E "s|^( *vendorHash = \")sha256-[A-Za-z0-9+/=]*(\";)|\1${hash}\2|" flake.nix
}

# 古い hash のままだと fixed-output derivation が store から再利用されうるので、必ず偽の hash で取り直す
set_vendor_hash "$FAKE_HASH"
if build_log=$(nix build .#default --no-link -L 2>&1); then
  printf 'ERROR: 偽の vendorHash でビルドが通りました\n' >&2
  exit 1
fi
got=$(printf '%s\n' "$build_log" | sed -n -E 's/^ *got: *(sha256-[A-Za-z0-9+/=]+).*/\1/p' | head -n 1)
if [ -z "$got" ]; then
  printf 'ERROR: nix build の出力に got: がありません\n%s\n' "$build_log" >&2
  exit 1
fi
set_vendor_hash "$got"

# 付け直した hash で通ることを確かめる
nix build .#default --no-link -L

printf '%s を上げ、vendorHash を %s にしました\n' "$MODULE" "$got"
write_output true
