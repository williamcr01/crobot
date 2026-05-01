package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

func packPtrLen(ptr uint32, length uint32) uint64 {
	return uint64(ptr)<<32 | uint64(length)
}

func unpackPtrLen(v uint64) (uint32, uint32) {
	return uint32(v >> 32), uint32(v)
}

func writeJSON(ctx context.Context, mod api.Module, malloc api.Function, v any) (uint32, uint32, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, 0, err
	}
	return writeBytes(ctx, mod, malloc, data)
}

func writeBytes(ctx context.Context, mod api.Module, malloc api.Function, data []byte) (uint32, uint32, error) {
	results, err := malloc.Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, 0, fmt.Errorf("malloc: %w", err)
	}
	if len(results) == 0 {
		return 0, 0, fmt.Errorf("malloc returned no result")
	}
	ptr := uint32(results[0])
	if len(data) > 0 && !mod.Memory().Write(ptr, data) {
		return 0, 0, fmt.Errorf("memory write failed at %d len %d", ptr, len(data))
	}
	return ptr, uint32(len(data)), nil
}

func readBytes(ctx context.Context, mod api.Module, free api.Function, ptr uint32, length uint32) ([]byte, error) {
	data, err := readBytesNoFree(mod, ptr, length)
	if err != nil {
		return nil, err
	}
	if _, err := free.Call(ctx, uint64(ptr)); err != nil {
		return nil, fmt.Errorf("free: %w", err)
	}
	return data, nil
}

func readBytesNoFree(mod api.Module, ptr uint32, length uint32) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	data, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return nil, fmt.Errorf("memory read failed at %d len %d", ptr, length)
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func callJSONInJSONOut(ctx context.Context, p *Plugin, fn api.Function, input any, output any) error {
	inPtr, inLen, err := writeJSON(ctx, p.Module, p.Functions.Malloc, input)
	if err != nil {
		return err
	}
	defer p.Functions.Free.Call(ctx, uint64(inPtr))

	results, err := fn.Call(ctx, uint64(inPtr), uint64(inLen))
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("guest function returned no result")
	}
	outPtr, outLen := unpackPtrLen(results[0])
	data, err := readBytes(ctx, p.Module, p.Functions.Free, outPtr, outLen)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode guest JSON: %w", err)
	}
	return nil
}

func callJSONIn(ctx context.Context, p *Plugin, fn api.Function, input any) error {
	inPtr, inLen, err := writeJSON(ctx, p.Module, p.Functions.Malloc, input)
	if err != nil {
		return err
	}
	defer p.Functions.Free.Call(ctx, uint64(inPtr))
	_, err = fn.Call(ctx, uint64(inPtr), uint64(inLen))
	return err
}

func callStringStringJSONOut(ctx context.Context, p *Plugin, fn api.Function, a string, b any, output any) error {
	aPtr, aLen, err := writeBytes(ctx, p.Module, p.Functions.Malloc, []byte(a))
	if err != nil {
		return err
	}
	defer p.Functions.Free.Call(ctx, uint64(aPtr))

	bPtr, bLen, err := writeJSON(ctx, p.Module, p.Functions.Malloc, b)
	if err != nil {
		return err
	}
	defer p.Functions.Free.Call(ctx, uint64(bPtr))

	results, err := fn.Call(ctx, uint64(aPtr), uint64(aLen), uint64(bPtr), uint64(bLen))
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("guest function returned no result")
	}
	outPtr, outLen := unpackPtrLen(results[0])
	data, err := readBytes(ctx, p.Module, p.Functions.Free, outPtr, outLen)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, output)
}
