# Plan-to-issues workflow

Read [README.md](README.md) first. Input is a parent Issue and an approved
`plan.md` containing one `## Implementation Work` section.

1. Use the supplied parent Issue and current checkout as context, and read the
   relevant approved Plan.
2. Read each `###` subsection under `## Implementation Work` as one proposed
   child Issue. Use its heading as the title and its scope, dependencies, and
   acceptance evidence as the body.
3. List the parent's open and closed native sub-issues. Immediately before each
   create, skip the proposal when the same work is already represented.
4. Create each missing Issue and attach it to the parent through GitHub's native
   sub-issue API. Do not create an ordinary unparented Issue as a fallback.
5. Do not update or close existing children unless the user explicitly names
   those Issues.
6. After all missing children are attached, remove `Next` from the parent's SDD
   summary and stop. Do not change repository files or open a PR.

If Issue creation or native sub-issue attachment is unavailable, stop before
creating anything and report the missing capability.
