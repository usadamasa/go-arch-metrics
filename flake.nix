{
  description = "go-arch-metrics の CLI (packages) と開発環境 (devShells)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      # devShell の go は nixpkgs の Go 製ツール (gosec など) をビルドした系列 (pkgs.go = go_1_26) に揃える。
      # 新しい系列の go を並べると、gosec が export data を読めず SSA の解析を飛ばして exit 1 になる。
      # nixos-unstable の go_1_26 は 1.26.9 で、crypto/x509 の脆弱性 (GO-2026-6809、1.26.10 で修正) が
      # cmd/analyze-modularity から届くため vulncheck が exit 3 を返す。channel に 1.26.10 が降りるまで、
      # go だけソースを差し替える (#33)。
      # 差し替えは channel の go_1_26 が 1.26.10 より古いときだけ効く。Dependabot の nix PR が 1.26.10 以上の
      # nixpkgs を取り込んだ時点で channel の go_1_26 に戻り、以後のパッチ更新は flake.lock と一緒に上がる。
      # 戻った後の差し替えの分岐は使われないので、そのとき消す
      devGo =
        pkgs:
        if pkgs.lib.versionOlder pkgs.go_1_26.version "1.26.10" then
          pkgs.go_1_26.overrideAttrs (old: rec {
            version = "1.26.10";
            src = pkgs.fetchurl {
              url = "https://go.dev/dl/go${version}.src.tar.gz";
              hash = "sha256-/WPog7ovK/ypPBZGsx6+9j4rHk0rMXyXidl2po2B828=";
            };
          })
        else
          pkgs.go_1_26;
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          # vulncheck は自分をビルドした Go より新しい go 行のモジュールを測れない
          # (package requires newer Go version)。pkgs.go は一つ前の系列に留まるので go_latest でビルドする。
          # 利用側が nixpkgs を follows させれば、利用側の nixpkgs の go_latest でビルドされる
          buildGoModule = pkgs.buildGoModule.override { go = pkgs.go_latest; };
        in
        {
          default = buildGoModule {
            pname = "go-arch-metrics";
            # 版の実体は plugin.json (tagpr が上げる)
            version = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
            src = self;
            # go.mod / go.sum を変えたら付け直す。CI の nix build が hash mismatch の got: で正しい値を出す
            vendorHash = "sha256-lAik/wRbmvSPkZe8XiXUCUp13BQiD97yWy57fvaF0H0=";
            subPackages = [
              "cmd/analyze-arch-lint"
              "cmd/analyze-modularity"
              "cmd/vulncheck"
            ];
            # テストはこのリポジトリの CI (go test) が持つ。利用側がこの package を参照しても
            # binary cache が無いので手元でビルドされ、checkPhase も利用側で走る。Linux の build sandbox は
            # ネットワークも git も無く、TestVulncheck と analyze-modularity の git を使うテストが落ちる
            doCheck = false;
            # 実行時の go・git・go-arch-lint は利用側の PATH のものを使う。go を包むと、対象プロジェクトの
            # go.mod とは別の版で go list が走るので包まない
            meta = {
              description = "Go アーキテクチャメトリクスの CLI (analyze-modularity, analyze-arch-lint, vulncheck)";
              homepage = "https://github.com/usadamasa/go-arch-metrics";
              license = pkgs.lib.licenses.mit;
            };
          };
        }
      );

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShellNoCC {
          # analyze-* と vulncheck はこのリポジトリ自身のソースを Taskfile が go run するので含めない
          packages =
            [
              # go.mod の go 行と同じ系列。ずれたら GOTOOLCHAIN=local で即座に失敗させる
              (devGo pkgs)
            ]
            ++ (with pkgs; [
              go-task
              golangci-lint
              go-arch-lint
              gosec
              bats
              shellcheck
            ]);
          env.GOTOOLCHAIN = "local";
        };
      });
    };
}
