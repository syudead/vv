package domain

import (
	"errors"
	"net"
	"strconv"
)

// ErrListenFailed は、LAN からの接続の切り替えで新しいアドレスの待ち受けを開けなかった
// ことを表す。待ち受けは元のアドレスへ戻し、保存値は変えていない
// （specs/037-windows-app/contracts/network-settings-api.md §3）。
var ErrListenFailed = errors.New("cannot listen on the new address")

// NetworkSettings は Windows デスクトップ版の LAN からの接続の設定である
// （specs/037-windows-app/research.md R-14）。
type NetworkSettings struct {
	// LANAccess は保存した選択。保存値が無ければ偽。
	LANAccess bool
	// Port は今の待ち受けのポート。
	Port int
	// Addresses は LANAccess が真のときだけ、上がっている非ループバックの IPv4 アドレス
	// ごとの "http://<アドレス>:<ポート>/"。偽なら空。
	Addresses []string
}

// LANListenAddr は LAN からの接続の許可に合う待ち受けのアドレスを返す。許可するなら
// すべてのアドレス（0.0.0.0）、しないならループバック（127.0.0.1）だけで待ち受ける。
func LANListenAddr(lanAccess bool, port int) string {
	host := "127.0.0.1"
	if lanAccess {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
