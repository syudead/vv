import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// The only caller of the screen's /api/peer operations; the Pairing page never calls fetch.

export type PeerState = components["schemas"]["PeerState"];
export type PeerLink = components["schemas"]["PeerLink"];
export type PeerGrant = components["schemas"]["PeerGrant"];
export type PairingCode = components["schemas"]["PairingCode"];
export type PairPeerRequest = components["schemas"]["PairPeerRequest"];

const jsonHeaders = { "Content-Type": "application/json" };

export function getPeer(signal?: AbortSignal): Promise<PeerState> {
  return request<PeerState>("/api/peer", { signal });
}

/** Replaces an unused code; a node another VVMDM already reads answers 409 already_paired. */
export function issuePairingCode(signal?: AbortSignal): Promise<PairingCode> {
  return request<PairingCode>("/api/peer/code", { method: "POST", signal });
}

/** Answers the connected state; certificate_untrusted carries the fingerprint to send back. */
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

export function updatePeerAddress(
  address: string,
  signal?: AbortSignal,
): Promise<PeerState> {
  return request<PeerState>("/api/peer/address", {
    method: "PUT",
    headers: jsonHeaders,
    body: JSON.stringify({ address }),
    signal,
  });
}

/** Sends the fingerprint the screen showed; another one waiting answers 409 fingerprint_changed. */
export function confirmPeerCertificate(
  fingerprint: string,
  signal?: AbortSignal,
): Promise<PeerState> {
  return request<PeerState>("/api/peer/certificate", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ fingerprint }),
    signal,
  });
}

async function deletePeerPart(path: string, signal?: AbortSignal): Promise<void> {
  const response = await apiFetch(path, { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}

/** Stops reading the other VVMDM, and also clears a revoked state; a missing link answers 204 too. */
export function unpairPeer(signal?: AbortSignal): Promise<void> {
  return deletePeerPart("/api/peer/link", signal);
}

/** Withdraws the reading VVMDM's access; a missing grant answers 204 too. */
export function revokePeerGrant(signal?: AbortSignal): Promise<void> {
  return deletePeerPart("/api/peer/grant", signal);
}
