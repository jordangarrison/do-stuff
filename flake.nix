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
        { pkgs, self', ... }:
        {
          packages.default = pkgs.buildGoModule {
            pname = "do-stuff";
            version = "0.0.0";
            src = ./.;
            vendorHash = null; # no Go dependencies in bootstrap

            subPackages = [ "cmd/ds" ];

            ldflags = [
              "-s"
              "-w"
              "-X main.version=v0.0.0"
            ];

            doCheck = true;

            meta = with pkgs.lib; {
              description = "Task-based multi-repo worktree manager";
              homepage = "https://github.com/jordangarrison/do-stuff";
              license = licenses.mit;
              mainProgram = "ds";
              platforms = platforms.unix;
            };
          };

          apps.default = {
            type = "app";
            program = "${self'.packages.default}/bin/ds";
          };

          checks.package = self'.packages.default;

          devShells.default = pkgs.mkShell {
            inputsFrom = [ self'.packages.default ];
            packages = with pkgs; [
              # Compile-time
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

      flake = {
        overlays.default = _final: prev: {
          do-stuff = prev.callPackage (
            { buildGoModule }:
            buildGoModule {
              pname = "do-stuff";
              version = "0.0.0";
              src = ./.;
              vendorHash = null;
              subPackages = [ "cmd/ds" ];
              ldflags = [
                "-s"
                "-w"
                "-X main.version=v0.0.0"
              ];
              meta.mainProgram = "ds";
            }
          ) { };
        };
      };
    };
}
