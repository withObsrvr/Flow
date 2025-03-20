# Flow Distribution Strategy

This document outlines the distribution strategy for Flow and its plugins, ensuring compatibility and ease of use across different environments.

## Distribution Methods

Flow and its plugins are distributed through multiple channels:

### 1. GitHub Releases

Pre-built binaries are available on GitHub Releases, containing:
- Flow executables (`flow`, `graphql-api`, `schema-registry`)
- Plugin shared objects (`.so` files)
- Documentation and checksums

These releases are built using Nix to ensure reproducibility and consistent toolchain versions.

### 2. Docker Images

Docker images are available on Docker Hub with all components pre-installed:
```
docker pull withobsrvr/flow:latest
```

The Docker images contain the Flow executables and all plugins in the appropriate locations.

### 3. Nix Flakes

For users of Nix, we provide flakes for building Flow and its plugins:

```bash
# Install the complete Flow distribution
nix profile install github:withObsrvr/flow

# Build from source
nix build github:withObsrvr/flow
```

For a complete distribution including all plugins, use the meta-flake:

```bash
nix build github:withObsrvr/flow#meta
```

## Go Plugin Compatibility

Go plugins require exact compatibility between the host application and the plugins. To ensure this:

1. All components are built with the same Go version (currently 1.23)
2. All binaries in a release are built by the same workflow
3. The Nix builds ensure consistent toolchain versions

**Important:** Always use plugins with the Flow version they were built for. Mixing plugins from different releases may cause runtime errors or crashes.

## Development and Testing

For development, we recommend using the meta-flake's development shell:

```bash
nix develop github:withObsrvr/flow#meta
```

This provides a complete development environment with all the necessary dependencies.

## Release Process

1. New tags (e.g., `v0.1.0`) trigger the GitHub Actions workflow
2. The workflow builds Flow and all plugins with the same toolchain
3. A release is created with all binaries and documentation
4. A Docker image is built and pushed to Docker Hub

## Adding New Plugins

To add a new plugin to the distribution:

1. Ensure the plugin has a working Nix build
2. Add the plugin to the GitHub workflow in `.github/workflows/build-and-release.yml`
3. Add the plugin to the meta-flake in `meta-flake.nix`
4. Update documentation to mention the new plugin

## Versioning

We follow semantic versioning:
- Major version: Breaking changes to the Flow API
- Minor version: New features, non-breaking changes
- Patch version: Bug fixes and minor improvements

Plugin version numbers should align with the Flow version they are compatible with. 