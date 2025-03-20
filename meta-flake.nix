{
  description = "Obsrvr Flow with all plugins";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    
    # Main Flow repository
    flow = {
      url = "github:withObsrvr/flow";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    
    # Flow plugins
    flow-consumer-sqlite = {
      url = "github:withObsrvr/flow-consumer-sqlite";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    
    flow-processor-latestledger = {
      url = "github:withObsrvr/flow-processor-latestledger";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    
    flow-source-bufferedstorage-gcs = {
      url = "github:withObsrvr/flow-source-bufferedstorage-gcs";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs = { self, nixpkgs, flake-utils, flow, flow-consumer-sqlite, 
              flow-processor-latestledger, flow-source-bufferedstorage-gcs }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        # Access packages from all inputs
        flowPackage = flow.packages.${system}.default;
        sqlitePlugin = flow-consumer-sqlite.packages.${system}.default;
        latestLedgerPlugin = flow-processor-latestledger.packages.${system}.default;
        gcsPlugin = flow-source-bufferedstorage-gcs.packages.${system}.default;
      in
      {
        packages = {
          default = pkgs.symlinkJoin {
            name = "flow-complete";
            paths = [
              flowPackage
              sqlitePlugin
              latestLedgerPlugin
              gcsPlugin
            ];
            
            # Create a clean directory structure
            postBuild = ''
              mkdir -p $out/plugins
              
              # Move plugin .so files to plugins directory
              find $out/lib -name "*.so" -exec mv {} $out/plugins/ \;
              
              # Clean up empty directories
              find $out -type d -empty -delete
              
              # Create version documentation
              mkdir -p $out/doc
              cat > $out/doc/README.md << EOF
              # Flow Complete Distribution
              
              This package contains the Flow application and the following plugins:
              
              - SQLite Consumer
              - Latest Ledger Processor
              - GCS Buffered Storage Source
              
              All components were built with the same Go toolchain (${pkgs.go_1_23.version}) to ensure compatibility.
              
              ## Directory Structure
              
              - \`bin/\` - Flow executables
              - \`plugins/\` - Plugin files (.so)
              
              ## Version Information
              
              - Flow: $(cat ${flowPackage}/VERSION 2>/dev/null || echo "unknown")
              - Built with Go ${pkgs.go_1_23.version}
              - Built on: $(date)
              
              EOF
            '';
          };
          
          # Also expose individual components
          flow = flowPackage;
          sqlite-plugin = sqlitePlugin;
          latestledger-plugin = latestLedgerPlugin;
          gcs-plugin = gcsPlugin;
        };
        
        # Development shell with all tools needed for the project
        devShell = pkgs.mkShell {
          buildInputs = [
            pkgs.go_1_23
            pkgs.sqlite
            pkgs.protobuf
          ];
          
          shellHook = ''
            echo "Flow development environment with all plugins"
            echo "Go version: $(go version)"
            echo ""
            echo "Available components:"
            echo "- Flow: ${flowPackage}"
            echo "- SQLite Consumer: ${sqlitePlugin}"
            echo "- Latest Ledger Processor: ${latestLedgerPlugin}"
            echo "- GCS Storage Source: ${gcsPlugin}"
            echo ""
            echo "To build everything: nix build"
            export GO111MODULE="on"
          '';
        };
      }
    );
} 