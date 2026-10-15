// Package wasmsection は WebAssembly モジュールのカスタムセクションを符号化する。作品情報プラグインの
// マニフェスト（vv.plugin）を足すテストの組み立て（plugintest）と、配布物を組む scripts/pluginpack が
// 同じ符号化を使う。
package wasmsection

import "encoding/binary"

// Custom は名前 name、中身 data のカスタムセクション（id 0）を符号化する。
func Custom(name string, data []byte) []byte {
	payload := binary.AppendUvarint(nil, uint64(len(name)))
	payload = append(payload, name...)
	payload = append(payload, data...)
	section := []byte{0}
	section = binary.AppendUvarint(section, uint64(len(payload)))
	return append(section, payload...)
}
