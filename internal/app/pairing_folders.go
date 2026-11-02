package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// refreshFolders reads the media folders of the node of link and reconciles that link's snapshot. A failure keeps
// the snapshot as it was; the next stream open or mediaFolders event reads it again. announce publishes
// domain.MediaFoldersChanged even when no row changed, since the link's lists did.
func (p *Pairing) refreshFolders(ctx context.Context, link domain.PeerLink, announce bool) {
	listed, err := p.client.MediaFolders(ctx, peerTarget(link), link.Secret)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Info("could not read the media folders of the read VVMDM",
				slog.String("peer", link.Name), slog.Any("error", err))
		}
		return
	}
	changed, err := p.store.ReplacePeerFolders(ctx, link.NodeID, listed, p.now())
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		p.logger.Warn("could not save the media folders of the read VVMDM",
			slog.String("peer", link.Name), slog.Any("error", err))
		return
	}
	if changed || announce {
		p.publish(domain.MediaFoldersChanged{ViewOnly: true})
	}
}

// LinkFolders answers the snapshot of the media folders of the link of nodeID with that link's state, whether or
// not it is connected; without that link it answers domain.ErrNotFound.
func (p *Pairing) LinkFolders(ctx context.Context, nodeID string) (domain.PeerLinkFolders, error) {
	link, found, err := p.linkByNode(ctx, nodeID)
	if err != nil {
		return domain.PeerLinkFolders{}, err
	}
	if !found {
		return domain.PeerLinkFolders{}, domain.ErrNotFound
	}
	folders, err := p.store.PeerFolders(ctx, nodeID)
	if err != nil {
		return domain.PeerLinkFolders{}, err
	}
	status := p.loadStatus(link)
	return domain.PeerLinkFolders{State: status.state, Reason: status.reason, Folders: folders}, nil
}

// SetLinkFolderEnabled switches one of the link's folders on this node only; the link is never told. A folder the
// snapshot of nodeID lacks answers domain.ErrNotFound. A change publishes domain.MediaFoldersChanged.
func (p *Pairing) SetLinkFolderEnabled(
	ctx context.Context, nodeID string, folderID int64, enabled bool,
) (domain.PeerMediaFolder, error) {
	folder, changed, err := p.store.SetPeerFolderEnabled(ctx, nodeID, folderID, enabled)
	if err != nil {
		return domain.PeerMediaFolder{}, err
	}
	if changed {
		p.publish(domain.MediaFoldersChanged{ViewOnly: true})
	}
	return folder, nil
}

// ExcludedFolders returns the link's own ids of the folders of the link in slot that are switched off on this
// node, which every request to that link carries. A slot without a link answers domain.ErrNotFound.
func (p *Pairing) ExcludedFolders(ctx context.Context, slot int64) ([]int64, error) {
	link, found, err := p.findLink(ctx, func(link domain.PeerLink) bool { return link.Slot == slot })
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.ErrNotFound
	}
	folders, err := p.store.PeerFolders(ctx, link.NodeID)
	if err != nil {
		return nil, err
	}
	return domain.SwitchedOffFolders(folders), nil
}
