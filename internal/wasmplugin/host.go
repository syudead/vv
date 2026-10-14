package wasmplugin

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// hostUnavailable は vv のホスト関数が今は使えないことを表す値である。HTTP の実装が入るまで、
// どの関数もこれを返す（contracts/plugin-contract.md の Host functions）。
const hostUnavailable int32 = -1

// hostModule は vv のホストモジュール（http_send、http_meta、http_body）を組み立てる。
func hostModule(rt wazero.Runtime) wazero.HostModuleBuilder {
	i32 := api.ValueTypeI32
	unavailable := api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
		stack[0] = api.EncodeI32(hostUnavailable)
	})
	b := rt.NewHostModuleBuilder(vvModule)
	b.NewFunctionBuilder().
		WithGoModuleFunction(unavailable, []api.ValueType{i32, i32}, []api.ValueType{i32}).
		WithParameterNames("req_ptr", "req_len").
		Export("http_send")
	for _, name := range []string{"http_meta", "http_body"} {
		b.NewFunctionBuilder().
			WithGoModuleFunction(unavailable, []api.ValueType{i32, i32, i32}, []api.ValueType{i32}).
			WithParameterNames("handle", "buf_ptr", "buf_cap").
			Export(name)
	}
	return b
}
