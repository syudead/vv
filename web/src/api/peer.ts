import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";
import { clearListSnapshot } from "./listSnapshot";

// The only caller of the screen's /api/peer operations; the Pairing page never calls fetch.

export type PeerState = components["schemas"]["PeerState"];
export type PeerLinkState = components["schemas"]["PeerLinkState"];
export type PeerGrant = components["schemas"]["PeerGrant"];
export type PairingCode = components["schemas"]["PairingCode"];
export type PairPeerRequest = components["schemas"]["PairPeerRequest"];
export type PeerMediaFolder = components["schemas"]["PeerMediaFolder"];
export type PeerLinkMediaFolders = components["schemas"]["PeerLinkMediaFolders"];

const jsonHeaders = { "Content-Type": "application/json" };

export function getPeer(signal?: AbortSignal): Promise<PeerState> {
  return request<PeerState>("/api/peer", { signal });
}

/** Replaces an unused code; a node another VVMDM already reads answers 409 already_paired. */
export function issuePairingCode(signal?: AbortSignal): Promise<PairingCode> {
  return request<PairingCode>("/api/peer/code", { method: "POST", signal });
}

/**
 * Answers the whole state with the new link connected; certificate_untrusted carries the fingerprint to send back,
 * and peer_limit refuses a ninth link.
 */
export function pairPeer(
  body: PairPeerRequest,
  signal?: AbortSignal,
): Promise<PeerState> {
  return request<PeerState>("/api/peer/pair", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify(body),
    signal,
  });
}

function linkPath(nodeId: string, suffix = ""): string {
  return `/api/peer/links/${encodeURIComponent(nodeId)}${suffix}`;
}

export function updatePeerAddress(
  nodeId: string,
  address: string,
  signal?: AbortSignal,
): Promise<PeerState> {
  return request<PeerState>(linkPath(nodeId, "/address"), {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ address }),
    signal,
  });
}

/** Sends the fingerprint the screen showed for the link; another one waiting answers 409 fingerprint_changed. */
export function confirmPeerCertificate(
  nodeId: string,
  fingerprint: string,
  signal?: AbortSignal,
): Promise<PeerState> {
  return request<PeerState>(linkPath(nodeId, "/certificate"), {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ fingerprint }),
    signal,
  });
}

/** This node's snapshot of the link's folders with their switches, answered while the link is unreachable too. */
export function listPeerLinkMediaFolders(
  nodeId: string,
  signal?: AbortSignal,
): Promise<PeerLinkMediaFolders> {
  return request<PeerLinkMediaFolders>(linkPath(nodeId, "/media-folders"), { signal });
}

/** Changes this node's switch only; the link itself is never written. */
export async function setPeerLinkMediaFolderEnabled(
  nodeId: string,
  folderId: number,
  enabled: boolean,
  signal?: AbortSignal,
): Promise<PeerMediaFolder> {
  const updated = await request<PeerMediaFolder>(
    linkPath(nodeId, `/media-folders/${String(folderId)}/enabled`),
    { method: "PUT", headers: jsonHeaders, body: JSON.stringify({ enabled }), signal },
  );
  clearListSnapshot();
  return updated;
}

async function deletePeerPart(path: string, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(path, { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Stops reading one VVMDM, and also clears its revoked state; a missing link answers 204 too. */
export function unpairPeer(nodeId: string, signal?: AbortSignal): Promise<void> {
  return deletePeerPart(linkPath(nodeId), signal);
}

/** Withdraws the reading VVMDM's access; a missing grant answers 204 too. */
export function revokePeerGrant(signal?: AbortSignal): Promise<void> {
  return deletePeerPart("/api/peer/grant", signal);
}
