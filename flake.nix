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
        
        # Function to build a WASM plugin
        buildWasmPlugin = name: src: pkgs.stdenv.mkDerivation {
          inherit name src;
          buildInputs = [ pkgs.tinygo ];
          buildPhase = ''
            tinygo build -o ${name}.wasm -target=wasi ./main.go
          '';
          installPhase = ''
            mkdir -p $out
            cp ${name}.wasm $out/
          '';
        };
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow";
            version = "0.1.0";
            src = ./.;
            # Use the actual hash value provided by the build process
            vendorHash = "sha256-HbDWADDLpN7TPu3i0RqaOwBQgRkGP7rHp9T7IylsgwQ=";
            # Use -mod=mod to download modules directly from network
            buildFlags = ["-mod=mod"];
            # Specify the main packages to build
            subPackages = [ 
              "cmd/flow" 
              "cmd/graphql-api"
              "cmd/schema-registry"
            ];
          };
          
          # Example WASM plugin (for demonstration purposes)
          # zeromq-wasm = buildWasmPlugin "flow-consumer-zeromq" ./plugins/flow-consumer-zeromq;
        };

        devShell = pkgs.mkShell {
          buildInputs = [ 
            pkgs.go_1_23
            pkgs.tinygo # Add TinyGo for WASM compilation
          ];
        };
      }
    );
}
