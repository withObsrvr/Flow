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
        };

        devShell = pkgs.mkShell {
          buildInputs = [ pkgs.go_1_23 ];
        };
      }
    );
}
