# Quickstart: Three VVMDMs, one library, switches per folder

These steps prove the acceptance criteria that need three nodes or a second
node's screens, which `task check` and the two-link e2e do not cover.

## Prerequisites

- Three builds of this branch on one LAN, each with its own data directory
  and one media folder holding a video tagged `猫`
  ([running-vv.md](../../docs/how-to/running-vv.md)); call them A (the
  reader), B and C.
- A's owner account signed in, and B's and C's Pairing pages open.

## Steps

| Step | Expected result | Acceptance |
| --- | --- | --- |
| On A, pair B, then pair C from the same Pairing page | Both links listed as `connected`; A's library shows A's, B's and C's videos, each remote one marked with its node's name | 1 |
| Open A's tag list and filter by `猫` | One `猫` with the count summed over three nodes; the filter lists videos of all three | 2 |
| Open Settings, `Media folders` on A | A's folders, then one section each for B and C with their folders and `connected` | 3 |
| Switch one of B's folders off on A | Its videos leave A's library, search, folders and home; B's other folders stay; B's own library is unchanged | 5, 6 |
| Switch one of A's folders off, then read A from B (after pairing B to A) | B still lists that folder's videos; A's own screens do not | 6 |
| Stop C | A's library and a video page answer at once with A's and B's items; C's section reads as not connected and keeps its switches | 7 |
| With C stopped, rename `猫` on A | Nothing changes on A or B; the error names C | 8 |
| Start C again | C's section returns to `connected` with the same switches | Edge case 1 |
| On C, replace the folder A switched off with another path | On A, C's section shows the new path with the switch on | Edge case 2 |
| Pair A to B a second time, and try a ninth link | `already_paired` and `peer_limit` shown as reasons | Edge cases |
