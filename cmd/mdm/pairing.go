package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/peer"
	"github.com/syudead/vv/internal/store"
)

// newPairing names this node by its host name, which the peer shows on its screens. The forwarder reaches
// the peer through the pairing's connection, so a failed forwarded request marks the peer unreachable.
func newPairing(
	peers *store.PeerStore, build domain.BuildInfo, publisher app.Publisher, logger *slog.Logger,
) (*app.Pairing, *peer.Forwarder) {
	name, err := os.Hostname()
	if err != nil || !domain.ValidPeerName(name) {
		logger.Warn("could not read a usable host name; the paired VVMDM shows this one as VVMDM", slog.Any("error", err))
		name = "VVMDM"
	}
	client := peer.New(peer.Options{})
	pairing := app.NewPairing(app.PairingOptions{
		Store: peers, Link: client, Version: build.Version, NodeName: name,
		Publisher: publisher, Logger: logger,
	})
	return pairing, peer.NewForwarder(client, pairing)
}

// startPairingConnection runs the connection to the peer until the returned stop, which waits for it.
func startPairingConnection(pairing *app.Pairing) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		pairing.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}
