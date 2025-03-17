{
  description = "Stellar Flow Data Pipeline";

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
            vendorHash = null; # Use the vendor directory
            proxyVendor = true; # Use the vendor directory
            # Skip tests during the build
            doCheck = false;
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
