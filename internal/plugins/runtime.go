package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func newRuntime(ctx context.Context) (wazero.Runtime, error) {
	rt := wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	return rt, nil
}

func instantiateModule(ctx context.Context, rt wazero.Runtime, wasmPath string, hostModule api.Module) (api.Module, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, err
	}
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, err
	}
	defer compiled.Close(ctx)

	cfg := wazero.NewModuleConfig().WithName(filepath.Base(wasmPath))
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil, err
	}
	if mod.ExportedFunction("malloc") == nil {
		mod.Close(ctx)
		return nil, fmt.Errorf("missing required export malloc")
	}
	if mod.ExportedFunction("free") == nil {
		mod.Close(ctx)
		return nil, fmt.Errorf("missing required export free")
	}
	if mod.ExportedFunction("describe") == nil {
		mod.Close(ctx)
		return nil, fmt.Errorf("missing required export describe")
	}
	_ = hostModule
	return mod, nil
}

func buildHostModule(ctx context.Context, rt wazero.Runtime, m *Manager) (api.Module, error) {
	return rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(m.hostLog).Export("host_log").
		NewFunctionBuilder().WithFunc(m.hostConfigGet).Export("host_config_get").
		NewFunctionBuilder().WithFunc(m.hostEnvGet).Export("host_env_get").
		NewFunctionBuilder().WithFunc(m.hostFileRead).Export("host_file_read").
		NewFunctionBuilder().WithFunc(m.hostFileWrite).Export("host_file_write").
		NewFunctionBuilder().WithFunc(m.hostToolCall).Export("host_tool_call").
		NewFunctionBuilder().WithFunc(m.hostSendMessage).Export("host_send_message").
		NewFunctionBuilder().WithFunc(m.hostGetCWD).Export("host_get_cwd").
		Instantiate(ctx)
}
