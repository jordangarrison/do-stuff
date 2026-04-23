{
  description = "do-stuff: task-based multi-repo worktree manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    systems.url = "github:nix-systems/default";
  };

  outputs =
    inputs@{ flake-parts, systems, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import systems;

      perSystem =
        { pkgs, ... }:
        {
          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              # Compile-time
              go
              gopls
              gofumpt
              golangci-lint
              gotools
              delve

              # Runtime (for local testing)
              git
              tmux
              fzf
              jq
              gum

              # Nix housekeeping
              nixpkgs-fmt
              deadnix

              # Release
              goreleaser
            ];
          };

          formatter = pkgs.nixpkgs-fmt;
        };
    };
}
