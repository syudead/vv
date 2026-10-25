import { apiFetch, request, toRequestFailed } from "./client";
import type { components } from "./gen/openapi";

// The only caller of the screen's /api/peer operations; the Pairing page never calls fetch.

export type PeerState = components["schemas"]["PeerState"];
export type PairingCode = components["schemas"]["PairingCode"];
export type PairPeerRequest = components["schemas"]["PairPeerRequest"];

const jsonHeaders = { "Content-Type": "application/json" };

export function getPeer(signal?: AbortSignal): Promise<PeerState> {
  return request<PeerState>("/api/peer", { signal });
}

/** Replaces an unused code; a paired node answers 409 already_paired. */
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

/** Unpairs, and also clears a revoked state; a missing peer answers 204 too. */
export async function unpairPeer(signal?: AbortSignal): Promise<void> {
  const response = await apiFetch("/api/peer", { method: "DELETE", signal });
  if (!response.ok) throw await toRequestFailed(response);
}
