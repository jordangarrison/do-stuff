{
  description = "do-stuff: task-based multi-repo worktree manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    git-hooks.url = "github:cachix/git-hooks.nix";
    systems.url = "github:nix-systems/default";
  };

  outputs =
    inputs@{ flake-parts, systems, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import systems;

      imports = [ inputs.git-hooks.flakeModule ];

      perSystem =
        { config, pkgs, self', ... }:
        {
          pre-commit = {
            check.enable = true;
            settings.hooks = {
              # Fast, every commit
              gofumpt = {
                enable = true;
                name = "gofumpt";
                entry = "${pkgs.gofumpt}/bin/gofumpt -l -w";
                language = "system";
                types = [ "go" ];
              };
              # golangci-lint needs `go` on PATH at run time. git-hooks.nix's built-in
              # hook doesn't export it, so nix flake check fails in the sandbox. Force
              # a wrapper that prepends go's bin to PATH.
              golangci-lint = {
                enable = true;
                pass_filenames = false;
                entry = pkgs.lib.mkForce (
                  let
                    script = pkgs.writeShellScript "precommit-golangci-lint" ''
                      export PATH="${pkgs.go}/bin:$PATH"
                      exec ${pkgs.golangci-lint}/bin/golangci-lint run ./...
                    '';
                  in
                  builtins.toString script
                );
              };
              govet = {
                enable = true;
                name = "go vet";
                entry = "${pkgs.go}/bin/go vet ./...";
                language = "system";
                pass_filenames = false;
                types = [ "go" ];
              };
              trim-trailing-whitespace.enable = true;
              end-of-file-fixer.enable = true;
              nixpkgs-fmt.enable = true;
              deadnix.enable = true;

              # Slow, pre-push only
              go-test = {
                enable = true;
                name = "go test";
                entry = "${pkgs.go}/bin/go test ./...";
                language = "system";
                pass_filenames = false;
                stages = [ "pre-push" ];
              };
              go-build = {
                enable = true;
                name = "go build";
                entry = "${pkgs.go}/bin/go build ./...";
                language = "system";
                pass_filenames = false;
                stages = [ "pre-push" ];
              };
            };
          };

          packages.default = pkgs.buildGoModule {
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
              gopls
              gofumpt
              golangci-lint
              gotools
              delve

              git
              tmux
              fzf
              jq
              gum

              nixpkgs-fmt
              deadnix
              pre-commit

              goreleaser
            ];

            shellHook = config.pre-commit.installationScript;
          };

          formatter = pkgs.nixpkgs-fmt;
        };

      flake = {
        overlays.default = _final: prev: {
          do-stuff = prev.callPackage
            (
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
            )
            { };
        };
      };
    };
}
