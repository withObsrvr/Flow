{
  description = "Obsrvr Flow Data Indexer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix.url = "github:tweag/gomod2nix";
    gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, flake-utils, gomod2nix }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ gomod2nix.overlays.default ];
        };
      in
      {
        packages = {
          default = pkgs.buildGoApplication {
            pname = "flow";
            version = "0.1.0";
            src = ./.;
            modules = ./gomod2nix.toml;
            subPackages = [ 
              "cmd/flow" 
              "cmd/graphql-api"
              "cmd/schema-registry"
            ];
          };
        };

        # Required utility to generate gomod2nix.toml
        apps.gomod2nix = {
          type = "app";
          program = "${gomod2nix.packages.${system}.default}/bin/gomod2nix";
        };

        devShell = pkgs.mkShell {
          buildInputs = [ 
            pkgs.go_1_23
            gomod2nix.packages.${system}.default
          ];
          # Helper script to generate gomod2nix.toml
          shellHook = ''
            echo "Use 'gomod2nix' to generate/update the gomod2nix.toml file"
          '';
        };
      }
    );
}
