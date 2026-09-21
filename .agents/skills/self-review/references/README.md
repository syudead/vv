# Self review procedure

Run this after the change is complete and the repository checks pass, and
before the push. Review the whole diff against the base branch, not the file
currently open.

## How to run it

Run the review in a fresh context when the tool allows it — a subagent, a
second session, or a new conversation given only the diff and the sources of
truth. The context that wrote the change carries its own justification, and a
reviewer that inherits the justification confirms it instead of testing it.

Two rules make the difference between a review that finds things and one that
agrees with itself.

- **Cite a source.** Every claim that the change is correct names a file and a
  line in an artifact or an existing contract. "This looks right" is not an
  answer; `spec.md:99-108 says X, and the code does X` is.
- **Enumerate, do not judge.** Where a check asks for call sites, states, or
  paths, list them with `grep` and walk the list. A reviewer asked whether a
  case is handled says yes. A reviewer asked to list every case finds the one
  that is missing.

## The five checks

### 1. Reconcile the diff against the sources of truth

For each behaviour the diff adds or changes, quote the line in `spec.md`,
`data-model.md`, `contracts/`, `ui-design.md`, `api/openapi.yaml`, or
`AGENTS.md` that governs it, and state whether the code matches. A behaviour
with no governing line is either missing from the artifact or outside the
change's scope; say which.

This is the largest category by far. The artifact was read when the work
started and the code was written from memory afterwards, so the drift is
invisible from inside the change.

### 2. Enumerate the call sites of every invariant the change touches

Name the invariants the change relies on — "the representative location
determines `videos.container`", "a queued job refers to a current location",
"the library matches the registered folder set". For each one, `grep` every
place that can break it and walk the whole list.

A reviewer reading a diff finds the instance in front of it. The author knows
the invariant, so the author can find all of them at once. An invariant caught
one call site at a time costs one round per call site.

### 3. Apply the change to state that already exists

Ask what happens to a database, a queue, or a cache that was created by the
previous version. Applied migrations do not re-run; rows written under the old
rules stay; jobs queued under the old rules get claimed under the new ones.

A change that is correct on an empty database and wrong on an existing one
passes every test in the repository.

### 4. Walk the failure paths

For each I/O call, loop, and multi-step operation the diff adds: what remains
if it fails halfway? Name what is left behind, whether the caller can tell, and
whether a retry is safe. Row iteration that ignores its error, a request whose
follow-up fetch fails, two writes without a transaction, and a response that
arrives after a later one all belong here.

### 5. Find the sources of truth this change makes stale

List the artifacts, examples, and other features' specifications that the
change contradicts. `README` and `quickstart` command examples, another
feature's spec, the design-document index, and the execution plan all go stale
silently.

Anything outside this feature's own directory is a separate change. Record it
and raise it; do not fix it in this pull request.

## What stops the push

- A finding in checks 1–4 that is a defect: fix it, re-run the repository
  checks, then push.
- A finding that contradicts an approved artifact: stop. The artifact is the
  maintainer's to revise, and rewriting it here turns one change into a
  feature-wide rewrite.
- A finding in check 5 outside this feature's directory: record it in the pull
  request body and continue. It does not block this push and does not belong
  in this diff.
