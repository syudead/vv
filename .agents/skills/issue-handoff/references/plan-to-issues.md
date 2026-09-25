# Plan-to-issues workflow

Read [README.md](README.md) first. Input is a parent Issue and an approved
`plan.md` containing one `## Implementation Work` section.

A child Issue created here is what `/speckit-implement` builds from, and nothing
downstream re-reads the plan on its own. So this stage does not transcribe: it
**writes each unit up for the person who will implement it**, in full sentences,
with the links and the ordering they need.

The plan does not have to carry that prose. It carries the decisions — what the
units are, what each one covers, what counts as done — and a human approved
those. Expanding an approved decision into a readable Issue is this stage's job.
Deciding something the plan did not decide is not: scope the plan does not
cover, acceptance evidence not derivable from it, or an answer to a question
the parent Issue still holds open. The test is whether you can point at where
in the parent Issue or the approved artifacts the statement comes from.

1. Use the supplied parent Issue and current checkout as context, and read the
   relevant approved Plan.
2. Read each `###` subsection under `## Implementation Work` as one proposed
   child Issue. Its heading is the Issue title.
3. **Write the Issue body from the approved artifacts.** Take the unit's scope,
   dependencies, and acceptance evidence, and expand them into something a
   person can pick up cold: what changes and where, what has to land first, and
   what counts as done. Draw the detail from the artifacts the plan points at —
   the contract for an endpoint's errors, the data model for a field's rules,
   the parent Issue for the behaviour — rather than leaving the implementer to
   find them.
   Include a link to `plan.md`, the feature directory, and the feature branch
   name.
4. **Do not decide anything new here.** For every statement in the body, you
   must be able to name where in the parent Issue, the plan, or an artifact it
   comes from. If writing a usable Issue would require settling something none
   of them settle — scope the plan does not cover, acceptance evidence that
   cannot be derived, or a question the parent Issue still holds open — do not
   invent it:
   put that question to the user. Where the answer belongs depends on what it
   settles, and neither one is written here.
   - It changes what the user gets — scope, an interaction, how data is
     protected — so it is a requirement: it goes into the parent Issue through
     the [`issue-spec` skill](../../issue-spec/SKILL.md). Rerun `plan` when the
     new requirement moves the breakdown or the acceptance evidence.
   - It is a structural or technical decision the plan should have made: it goes
     into `plan.md` through `/speckit-plan`
     ([P-5](../../../../docs/design-docs/plan-quality.md)).

   Create no child Issue until the artifact that owns the answer carries it and
   a human has approved that change. A child Issue built on an answer the parent
   Issue does not have leaves the specification behind the implementation.
5. List the parent's open and closed native sub-issues. Immediately before each
   create, skip the proposal when the same work is already represented.
6. Create each missing Issue and attach it to the parent through GitHub's native
   sub-issue API. Do not create an ordinary unparented Issue as a fallback.
7. Do not update or close existing children unless the user explicitly names
   those Issues.
8. After all missing children are attached, stop. Do not edit the parent body,
   change repository files, or open a PR.

If Issue creation or native sub-issue attachment is unavailable, stop before
creating anything and report the missing capability.
