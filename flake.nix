{
  description = "Obsrvr Flow Data Indexer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow";
            version = "0.1.0";
            src = ./.;
            # Use the actual hash value provided by the build process
            vendorHash = "sha256-HbDWADDLpN7TPu3i0RqaOwBQgRkGP7rHp9T7IylsgwQ=";
            
            
            
            # Set flags to explicitly use modules instead of vendor
            buildFlags = ["-mod=mod"];
            
            # Ensure these env vars are set during builds
            GOFLAGS = "-mod=mod";
            
            # Specify the main packages to build
            subPackages = [ 
              "cmd/flow" 
              "cmd/graphql-api"
              "cmd/schema-registry"
            ];
          };
        };

        devShell = pkgs.mkShell {
          buildInputs = [ 
            pkgs.go_1_23
          ];
          # Set environment variables in the development shell too
          shellHook = ''
            export GOFLAGS="-mod=mod"
          '';
        };
      }
    );
}
