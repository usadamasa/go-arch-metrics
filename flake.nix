{
  description = "go-arch-metrics の開発環境 (Go と lint・テストのツール)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (
        pkgs:
        let
          # nixos-unstable の go_1_26 は 1.26.8 で、net/http の脆弱性 (GO-2026-6617 ほか、1.26.9 で修正) が
          # cmd/vulncheck の HTTP クライアントから届くため vulncheck が exit 3 を返す。nixpkgs master には
          # 1.26.9 が入っている (NixOS/nixpkgs#571801) が channel に降りるまで、go だけソースを差し替える。
          # 他の Go 製ツールは nixpkgs の go でビルドされたキャッシュのまま。channel が追いついたら
          # この override を消してから nix flake update を打つ (#33)。nix flake update だけでは消えない
          go = pkgs.go_1_26.overrideAttrs (old: rec {
            version = "1.26.9";
            src = pkgs.fetchurl {
              url = "https://go.dev/dl/go${version}.src.tar.gz";
              hash = "sha256-lzXX3Ntls10/pXfwQGRzfAO4nPGitx5uaf4vPG+f1Mo=";
            };
          });
        in
        {
          default = pkgs.mkShellNoCC {
            # analyze-* と vulncheck はこのリポジトリ自身のソースを Taskfile が go run するので含めない
            packages =
              [
                # go.mod の go 行と同じ版。ずれたら GOTOOLCHAIN=local で即座に失敗させる
                go
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
        }
      );
    };
}
