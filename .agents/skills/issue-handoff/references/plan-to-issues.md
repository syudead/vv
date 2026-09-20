# Plan-to-issues workflow

Read [README.md](README.md) first. Input is a parent Issue and an approved
`plan.md` containing one `## Implementation Work` section.

A child Issue created here is what `/speckit-implement` builds from. Nothing
downstream re-reads the plan on its own, so a unit that is too thin to
implement produces a thin implementation. This stage copies the plan; it does
not improve it, and it does not paper over it.

1. Use the supplied parent Issue and current checkout as context, and read the
   relevant approved Plan.
2. Read each `###` subsection under `## Implementation Work` as one proposed
   child Issue. Use its heading as the title and its scope, dependencies, and
   acceptance evidence as the body.
3. **Check the units before creating anything.** For each proposed child, all of
   the following must already hold in the plan
   ([P-5](../../../../docs/design-docs/plan-quality.md)):
   - the heading works as an Issue title on its own, away from the plan
   - the scope names what changes, and points to the artifact section holding
     the detail where there is one
   - the acceptance evidence is observable — a check that passes, a response
     that is returned, something visible on screen. "Implemented correctly" is
     not evidence
   - each dependency names another unit in this same section
   - the units together cover the feature, and do not overlap

   Where a unit fails this, stop before creating any Issue and report which
   unit and which point. The fix belongs in `plan.md` through `/speckit-plan`.
   Never invent scope, dependencies, or acceptance evidence to fill the gap: an
   Issue is a durable instruction to whoever implements it, and inventing its
   content here puts work nobody approved into the feature.
4. Give each Issue body, after the copied text, a link to the plan it came from
   and the name of the feature branch, so the implementer can reach the spec and
   the artifacts without being told where they are.
5. List the parent's open and closed native sub-issues. Immediately before each
   create, skip the proposal when the same work is already represented.
6. Create each missing Issue and attach it to the parent through GitHub's native
   sub-issue API. Do not create an ordinary unparented Issue as a fallback.
7. Do not update or close existing children unless the user explicitly names
   those Issues.
8. After all missing children are attached, remove `Next` from the parent's SDD
   summary and stop. Do not change repository files or open a PR.

If Issue creation or native sub-issue attachment is unavailable, stop before
creating anything and report the missing capability.
