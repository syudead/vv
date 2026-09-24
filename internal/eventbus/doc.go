// Package eventbus は状態の変化（domain.Event）を購読者へ配る。
//
// 発行する側（internal/store・internal/app）はこのパッケージを import しない。
// それぞれが自分の宣言した Publish の interface 越しに発行し、*Bus を渡すのも、
// 購読者を登録するのも cmd/mdm だけである。そのため、購読者を1つ増やしても
// 発行する側は変わらない。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package eventbus
