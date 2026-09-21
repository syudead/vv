# Measuring this skill

The skill is worth its cost only if it reaches a useful fraction of what a
reviewer posts on the pull request. That fraction is measurable, because the
007 settings-screen feature left 73 classified review findings against known
commits.

## Method

1. Check out the first pushed commit of one of the pull requests below. That is
   the state the reviewer saw first.
2. Run the review against `main..<commit>` with no knowledge of the answers.
3. Count how many of that pull request's findings the review reproduces, and
   how many it raises that the reviewer did not.

Reproduction means naming the same defect, not the same wording. A finding that
names the right location and the wrong cause does not count.

The counts below are every finding the reviewer eventually posted on that pull
request, across all of its rounds — not only the ones it posted on the first
commit. That is deliberate: finding them in one pass is the point of running
the review before the push, and the reviewer needed up to four rounds to reach
them one at a time.

## Known answers

| First pushed commit | Pull request | Findings |
| --- | --- | --- |
| `d8f0a59` | #63 spec | 2 |
| `ac969de` | #64 plan | 6 |
| `d3a5d84` | #70 spec revision | 26 |
| `204da4c` | #72 design | 4 |
| `6d7199c` | #73 storage and scanning | 12 |
| `c8782fe` | #74 settings API | 6 |
| `97f03e8` | #75 settings UI | 4 |
| `f864026` | #77 reconciliation fixes | 5 |
| `229fb5b` | #79 final review gaps | 2 |
| `ac5aacd` | #76 integration validation | 1 |
| `3616eae` | #80 location generation | 1 |
| `ade2b9d` | #82 vite proxy and E2E | 2 |
| `bc5ec82` | #83 stale folder refresh | 1 |

These thirteen pull requests carry 72 of the 73 findings; the last one is on #85,
which is still open. `#70` is the useful case for artifact work and `#73` for
implementation. `#80` is a single finding and the most expensive one to miss:
it makes every existing database fail to start.

## Reading the result

100% is not the target. The reviewer still runs on the pull request, so this
skill only has to move findings earlier. Halving the findings that reach the
pull request halves the remediation rounds, and each round is one session.

Watch two numbers over time: the share of reviewer findings reproduced, and the
number of pull requests that need no remediation commit at all. The second is
what actually shortens the feature.
