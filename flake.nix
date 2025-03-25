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
            # Use vendoring with the correct hash
            vendorHash = "sha256-07UGAsWkSltp4gIJbFQWzVTpPS8yxiR9t2xcX44S6tk=";
            # Make sure we're using the vendor directory
            proxyVendor = true;
            # Skip go mod verification/download by using -mod=vendor 
            buildFlags = [ "-mod=vendor" ];
            # Set environment variables for go builds
            env = {
              GO111MODULE = "on";
            };
            # Ensure vendor directory is complete and correct before building
            preBuild = ''
              echo "Using vendor directory for building..."
              
              # Add a touch command to make sure the modified files are recognized
              touch internal/pluginmanager/wasm_loader.go
              touch internal/pluginmanager/loader.go
            '';
            # Specify the main packages to build
            subPackages = [ 
              "cmd/flow" 
              "cmd/graphql-api"
              "cmd/schema-registry"
            ];
            # Use Go 1.24.1
            go = pkgs.go_1_24;
          };
        };

        devShell = pkgs.mkShell {
          buildInputs = [ 
            pkgs.go_1_24
            # Include a modern version of make
            pkgs.gnumake
            # Standard build tools
            pkgs.gcc
          ];
          # Set a helpful shell configuration
          shellHook = ''
            echo "Flow development environment with Go 1.24"
            export GO111MODULE="on"
            echo "Note: TinyGo is not compatible with Go 1.24 yet. For WASM plugins, use standard Go WASM build."
          '';
        };
      }
    );
}
