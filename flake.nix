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

      # vulncheck は自分をビルドした Go より新しい go 行のモジュールを測れない
      # (package requires newer Go version)。nixpkgs の既定の go は一つ前の系列に留まるので go_latest を使う。
      # 利用側が nixpkgs を follows させれば、利用側の nixpkgs の go_latest でビルドされる
      goFor = pkgs: pkgs.go_latest;
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          buildGoModule = pkgs.buildGoModule.override { go = goFor pkgs; };
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
              # package と同じ go。go.mod の go 行より古ければ GOTOOLCHAIN=local で即座に失敗させる
              (goFor pkgs)
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
