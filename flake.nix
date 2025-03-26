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
              # Enable WASM debugging
              FLOW_DEBUG_WASM = "1";
              GOLOG_LEVEL = "debug";
            };
            # Ensure vendor directory is complete and correct before building
            preBuild = ''
              echo "Using vendor directory for building..."
              
              # Add a touch command to make sure the modified files are recognized
              touch internal/pluginmanager/wasm_loader.go
              touch internal/pluginmanager/loader.go
              
              # Print the contents of the files we modified to verify changes
              echo "Verifying wasm_loader.go contents..."
              grep -n "WASMProcessorPlugin" internal/pluginmanager/wasm_loader.go || echo "WASMProcessorPlugin not found"
              grep -n "loadProcessorWASM" internal/pluginmanager/wasm_loader.go || echo "loadProcessorWASM not found"
              
              # We'll copy the WASM file later in installPhase
            '';
            
            # Add a custom install phase to include the WASM module
            installPhase = ''
              runHook preInstall
              
              # Create default install directories
              mkdir -p $out/bin $out/plugins
              
              # Copy the compiled binaries
              cp -v $GOPATH/bin/* $out/bin/
              
              # Build a minimal real WASM module for testing...
              echo "Building a real WASM module for testing..."
              
              # Create a temp directory for our minimal WASM module
              TEMP_DIR=$(mktemp -d)
              
              # Create a minimal Go WASM module
              cat > $TEMP_DIR/main.go << EOF
package main

//export name
func _name() string {
	return "flow/processor/latest-ledger"
}

//export version
func _version() string {
	return "1.0.0"
}

//export initialize
func _initialize(configJSON string) int32 {
	return 0 // success
}

//export processLedger
func _processLedger(ledgerJSON string) string {
	return "{\"result\":\"ok\"}"
}

func main() {
	// WebAssembly modules don't have a main function
}
EOF
              
              # Build the WASM module
              cd $TEMP_DIR
              # Use the Go binary from nixpkgs instead of $GOPATH/bin/go
              GOOS=wasip1 GOARCH=wasm ${pkgs.go_1_24}/bin/go build -o flow-processor-latest-ledger.wasm main.go
              
              # Copy the WASM module to the output
              cp flow-processor-latest-ledger.wasm $out/plugins/
              chmod +x $out/plugins/flow-processor-latest-ledger.wasm
              
              # Cleanup
              cd -
              rm -rf $TEMP_DIR
              
              runHook postInstall
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
