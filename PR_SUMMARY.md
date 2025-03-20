# Add WebAssembly (WASM) Plugin Support

This PR adds support for WebAssembly (WASM) plugins in the Flow pipeline system, following the implementation plan outlined in `docs/flow_upgraded_implementation_plan.md`.

## Changes Overview

1. **Plugin Manager Refactoring**:
   - Created a `PluginLoader` interface to abstract the loading of different plugin types
   - Implemented `NativePluginLoader` for traditional `.so` plugins
   - Implemented `WASMPluginLoader` for WebAssembly `.wasm` plugins
   - Updated `PluginManager` to use the appropriate loader based on file extension

2. **WASM Runtime Integration**:
   - Added dependency on `github.com/tetratelabs/wazero` for WASM runtime
   - Implemented memory management for WASM plugins
   - Created a communication protocol for passing data between the host and WASM plugins

3. **Sample WASM Plugin**:
   - Created a sample WASM consumer plugin in `examples/wasm-plugin-sample`
   - Added a `Makefile` for building the WASM plugin with TinyGo
   - Created documentation for how to build and use WASM plugins

4. **Build System Updates**:
   - Updated `flake.nix` to include TinyGo for building WASM plugins
   - Added a function `buildWasmPlugin` to simplify building WASM plugins

5. **Documentation**:
   - Added comprehensive documentation for WASM plugin support in `docs/plugin_support.md`
   - Updated the main `README.md` to include information about WASM plugins
   - Created pipeline configuration examples using WASM plugins

6. **Testing**:
   - Added a test script `scripts/test_wasm_plugin.sh` to verify WASM plugin functionality

## Benefits of WASM Plugin Support

1. **Improved Security**: WASM plugins run in a sandboxed environment, reducing security risks compared to native plugins.

2. **Cross-Platform Compatibility**: WASM plugins can be built once and run on any platform that supports the WASM runtime.

3. **Language Flexibility**: While the initial implementation focuses on Go plugins compiled with TinyGo, the architecture allows for future support of plugins written in other languages.

4. **Safe Memory Management**: The WASM runtime provides memory isolation, preventing plugins from accessing the host's memory directly.

## How to Try It

1. Install TinyGo: `nix develop` (or follow TinyGo installation instructions)
2. Build the sample WASM plugin: `cd examples/wasm-plugin-sample && make build`
3. Copy the plugin to the plugins directory: `make install`
4. Run Flow with the WASM sample pipeline: `./flow run --config examples/pipelines/pipeline_wasm_sample.yaml`

## Future Work

1. Enhance the WASM plugin interface to fully support Source plugins
2. Add support for more complex data types in the communication protocol
3. Create tooling to simplify WASM plugin development
4. Implement plugin registry and versioning as outlined in the implementation plan 