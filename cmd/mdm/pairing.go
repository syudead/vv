package main

import (
	"log/slog"
	"os"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/peer"
	"github.com/syudead/vv/internal/store"
)

// newPairing names this node by its host name, which the peer shows on its screens.
func newPairing(peers *store.PeerStore, build domain.BuildInfo, logger *slog.Logger) *app.Pairing {
	name, err := os.Hostname()
	if err != nil || !domain.ValidPeerName(name) {
		logger.Warn("could not read a usable host name; the paired VVMDM shows this one as VVMDM", slog.Any("error", err))
		name = "VVMDM"
	}
	return app.NewPairing(app.PairingOptions{
		Store: peers, Link: peer.New(peer.Options{}), Version: build.Version, NodeName: name,
	})
}
