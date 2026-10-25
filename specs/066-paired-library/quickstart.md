# Quickstart: two VVMDMs on one LAN as one library

These steps prove what the in-process tests and the browser e2e cannot: a
pairing over HTTPS through a reverse proxy, a certificate change, and the
states a stopped or wiped peer shows after real restarts.

## Prerequisites

- Two VVMDM instances of the same release on one LAN, A and B, each with a
  media folder scanned and at least one video with a sidecar `.vtt`
  ([running-vv.md](../../docs/how-to/running-vv.md)).
- A reverse proxy in front of B with a self-signed certificate on
  `https://b.lan`, and B also reachable on `http://b.lan:8080`.
- The owner signed in on both; a second browser signed out, as a guest, on A.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| On A, Settings › Pairing › Show pairing code | A code and A's node name appear | 1 |
| On B, enter `http://a.lan:8080`, the code and B's address `https://b.lan`, press Pair | B shows `certificate_untrusted` with a fingerprint | Edge: certificate |
| Compare the fingerprint with the proxy's certificate and confirm | Both Settings pages show `connected`; A's library count equals A + B | 1, 2 |
| Open a video of B from A's library, play, seek and turn on the subtitle | Playback and the cues work; the card and the page carry B's name | 2, 9 |
| On A, change B's address to `http://b.lan:8080` | The state returns to `connected` without a new code | 3 |
| Change the owner password on B | A and B stay `connected`; A's library still lists B's videos | 5 |
| Stop B; on A unpair; start B | B's Settings page shows `revoked` (shown as unpaired by the other side); clearing it shows `unpaired` | 4 |
| Pair again; replace the proxy certificate; wait for the reconnect | A shows `unreachable` with reason `certificate` and the new fingerprint; confirming reconnects | Edge: certificate changed |
| Stop B | A's library drops B's videos, groups and smart folders and shows the notice; the guest browser shows it too | 11 |
| Start B | They return and the notice leaves | 11 |
| On B, make one video private; on A's guest browser, search it | Not listed; A's owner browser lists it | 13 |
| Wipe B's data directory, start B, open A | A shows `revoked`; pairing again from either side works | Edge: reinstalled |
