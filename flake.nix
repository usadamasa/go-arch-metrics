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

      # nixos-unstable の go_1_27 は 1.27.3 で、net/http の MaxBytesReader のデータ競合 (CVE-2026-97054) と
      # cgo 経由でビルド時に任意コードが実行される cmd/go などの脆弱性 (CVE-2026-97033〜97043) が残る。
      # 渡した Go が 1.27.4 より古いときだけソースを差し替える。Dependabot の nix PR が 1.27.4 以上の
      # nixpkgs を取り込んだら差し替えは外れ、以後のパッチ更新は flake.lock と一緒に上がる。そのときこの分岐ごと消す
      patchGo =
        pkgs: base:
        if pkgs.lib.versionOlder base.version "1.27.4" then
          base.overrideAttrs (old: rec {
            version = "1.27.4";
            src = pkgs.fetchurl {
              url = "https://go.dev/dl/go${version}.src.tar.gz";
              hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
            };
          })
        else
          base;
      devGo = pkgs: patchGo pkgs pkgs.go_1_27;
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          # vulncheck は自分をビルドした Go より新しい go 行のモジュールを測れない
          # (package requires newer Go version)。pkgs.go は一つ前の系列に留まるので go_latest でビルドする。
          # 利用側が nixpkgs を follows させれば、利用側の nixpkgs の go_latest でビルドされる。
          # go_latest が 1.27.4 より古いあいだは devShell と同じ差し替えを当てる
          buildGoModule = pkgs.buildGoModule.override { go = patchGo pkgs pkgs.go_latest; };
        in
        {
          default = buildGoModule {
            pname = "go-arch-metrics";
            # 版の実体は plugin.json (tagpr が上げる)
            version = (builtins.fromJSON (builtins.readFile ./.claude-plugin/plugin.json)).version;
            src = self;
            # go.mod / go.sum を変えたら付け直す。CI の nix build が hash mismatch の got: で正しい値を出す
            vendorHash = "sha256-FOuZs+MZV8quH0gsN13fJ8Xr/pM6b4ROTBOM94o+BLQ=";
            subPackages = [
              "cmd/analyze-arch-lint"
              "cmd/analyze-modularity"
              "cmd/arch-baseline"
              "cmd/vulncheck"
            ];
            # テストはこのリポジトリの CI (go test) が持つ。利用側がこの package を参照しても
            # binary cache が無いので手元でビルドされ、checkPhase も利用側で走る。Linux の build sandbox は
            # ネットワークも git も無く、TestVulncheck と analyze-modularity の git を使うテストが落ちる
            doCheck = false;
            # 実行時に呼ぶ go や lint ツールは利用側の PATH のものを使う。go を包むと、対象プロジェクトの
            # go.mod とは別の版で go list が走るので包まない
            meta = {
              description = "Go アーキテクチャメトリクスの CLI";
              homepage = "https://github.com/usadamasa/go-arch-metrics";
              license = pkgs.lib.licenses.mit;
            };
          };
        }
      );

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShellNoCC {
          # この plugin の CLI はこのリポジトリ自身のソースを go run するので含めない
          packages =
            [
              # go.mod の go 行と同じバージョン。ずれたら GOTOOLCHAIN=local で即座に失敗させる
              (devGo pkgs)
            ]
            ++ (with pkgs; [
              go-task
              golangci-lint
              go-arch-lint
              gosec
              shellcheck
            ]);
          env.GOTOOLCHAIN = "local";
        };
      });
    };
}
