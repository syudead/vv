# Tasks workflow

Read [README.md](README.md) first. Input is a parent Issue and a user request to
create or revise its task list.

1. Resolve the requested feature branch and feature directory. Read the
   available artifacts, including an existing `tasks.md`, as inputs.
2. Create an arbitrary-name sub-branch from the current feature branch.
3. Run the installed Spec Kit tasks procedure and then its analyze procedure.
   For UI Issues, treat `ui-design.md` as an input.
4. Each task ID is unique within this parent. Once child Issues have been
   created, existing IDs may not be reordered, renumbered, deleted, or reused.
   Append new tasks with IDs above the current maximum.
5. Push and open a feature-branch PR with `Refs #<parent>`. Stop.
6. After human merge, the maintainer marks Tasks complete and sets
   `Next: taskstoissues`.

After child Issues exist, represent a removed task as:

```markdown
- [x] ~~TNNN original task~~ (cancelled: reason)
```

Close its child Issue as not planned. A material requirement change gets a new
task ID; do not repurpose the old ID.

