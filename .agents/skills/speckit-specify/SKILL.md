---
name: "speckit-specify"
description: "Create or update the feature specification from a natural language feature description."
metadata:
  author: "github-spec-kit"
  source: "templates/commands/specify.md"
---

## Repository issue handoff

When this command is invoked with a GitHub parent Issue, read and follow
`.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/specify.md` first. Those
files own branch topology, GitHub preflight, PR scope, and stopping behavior.
This skill owns only Spec Kit artifact generation. A run creates or updates one
Spec PR and never starts Plan.


## Repository rules for specifications

Follow [docs/product-specs/spec-quality.md](../../../docs/product-specs/spec-quality.md)
(Q-1..Q-7). Where it conflicts with a general instruction below, **it wins**.

Two different people are meant below, and the words are not interchangeable.
The **requester** is whoever asked for this feature and is running this command;
they are the one a question goes to. The **user** is whoever will use the
finished product; the spec's User Scenarios are about them, and "what the user
gets" means their experience. A question never goes to them.
Two of its rules invert the defaults of this command:

- **Q-6** — an ambiguity you cannot resolve goes back to the requester as a question.
  A plausible interpretation is not a resolution.
- **Q-7** — an implementation constraint is never promoted into a product
  requirement or a scope boundary.

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Pre-Execution Checks

**Check for extension hooks (before specification)**:
- Check if `.specify/extensions.yml` exists in the project root.
- If it exists, read it and look for entries under the `hooks.before_specify` key
- If the YAML cannot be parsed or is invalid, do not skip silently: tell the user that `.specify/extensions.yml` could not be read (include the parser error) and that no hooks were checked, including any mandatory (`optional: false`) hooks registered there, then continue normally
- Filter out hooks where `enabled` is explicitly `false`. Treat hooks without an `enabled` field as enabled by default.
- For each remaining hook, do **not** attempt to interpret or evaluate hook `condition` expressions:
  - If the hook has no `condition` field, or it is null/empty, treat the hook as executable
  - If the hook defines a non-empty `condition`, skip the hook and leave condition evaluation to the HookExecutor implementation
- When constructing command invocations from hook command names, replace dots (`.`) with hyphens (`-`). For example, `speckit.git.commit` → `/speckit-git-commit`.
- For each executable hook, output the following based on its `optional` flag:
  - **Optional hook** (`optional: true`):
    ```
    ## Extension Hooks

    **Optional Pre-Hook**: {extension}
    Command: `/{command}`
    Description: {description}

    Prompt: {prompt}
    To execute: `/{command}`
    ```
  - **Mandatory hook** (`optional: false`):
    ```
    ## Extension Hooks

    **Automatic Pre-Hook**: {extension}
    Executing: `/{command}`
    EXECUTE_COMMAND: {command}

    Wait for the result of the hook command before proceeding to the Outline.
    ```
    After emitting the block above you MUST actually invoke the hook and wait for it to finish before continuing. Run it the same way you would run the command yourself in this agent/session (the invocation may differ from the literal `{command}` id shown above, e.g. a skills-mode agent runs it as `/skill:speckit-...` or `$speckit-...`). Emitting the block alone does not run the hook.
- If no hooks are registered or `.specify/extensions.yml` does not exist, skip silently

## Outline

The text the user typed after `/speckit-specify` in the triggering message **is** the feature description. Assume you always have it available in this conversation even if `$ARGUMENTS` appears literally below. Do not ask the user to repeat it unless they provided an empty command.

Given that feature description, do this:

1. **Generate a concise short name** (2-4 words) for the feature:
   - Analyze the feature description and extract the most meaningful keywords
   - Create a 2-4 word short name that captures the essence of the feature
   - Use action-noun format when possible (e.g., "add-user-auth", "fix-payment-bug")
   - Preserve technical terms and acronyms (OAuth2, API, JWT, etc.)
   - Keep it concise but descriptive enough to understand the feature at a glance
   - Examples:
     - "I want to add user authentication" → "user-auth"
     - "Implement OAuth2 integration for the API" → "oauth2-api-integration"
     - "Create a dashboard for analytics" → "analytics-dashboard"
     - "Fix payment processing timeout bug" → "fix-payment-timeout"

2. **Branch creation** (optional, via hook):

   If a `before_specify` hook ran successfully in the Pre-Execution Checks above, it will have created/switched to a git branch and output JSON containing `BRANCH_NAME` and `FEATURE_NUM`. Note these values for reference, but the branch name does **not** dictate the spec directory name.

   If the user explicitly provided `GIT_BRANCH_NAME`, pass it through to the hook so the branch script uses the exact value as the branch name (bypassing all prefix/suffix generation).

3. **Create the spec feature directory**:

   Specs live under the default `specs/` directory unless the user explicitly provides `SPECIFY_FEATURE_DIRECTORY`.

   **Resolution order for `SPECIFY_FEATURE_DIRECTORY`**:
   1. If the user explicitly provided `SPECIFY_FEATURE_DIRECTORY` (e.g., via environment variable, argument, or configuration), use it as-is
   2. Otherwise, auto-generate it under `specs/`:
      - Check `.specify/init-options.json` for `feature_numbering` (preferred) or `branch_numbering` (deprecated, migration only — will be removed in a future release)
      - If `"timestamp"`: prefix is `YYYYMMDD-HHMMSS` (current timestamp)
      - If `"sequential"` or absent: prefix is `NNN` (next available 3-digit number after scanning existing directories in `specs/`)
      - Construct the directory name: `<prefix>-<short-name>` (e.g., `003-user-auth` or `20260319-143022-user-auth`)
      - Set `SPECIFY_FEATURE_DIRECTORY` to `specs/<directory-name>`
      - If `branch_numbering` was used (and `feature_numbering` was absent), emit a one-line warning: "⚠️ `branch_numbering` in init-options.json is deprecated. Rename to `feature_numbering`."

   **Create the directory and spec file**:
   - `mkdir -p SPECIFY_FEATURE_DIRECTORY`
   - Resolve the active `spec-template` through the Spec Kit preset/template resolution stack (equivalent to `specify preset resolve spec-template`)
   - Copy the resolved `spec-template` file to `SPECIFY_FEATURE_DIRECTORY/spec.md` as the starting point
   - Set `SPEC_FILE` to `SPECIFY_FEATURE_DIRECTORY/spec.md`
   - Record the GitHub Issue that initiated this feature as `**Parent Issue**: #NNN` in
     `SPEC_FILE`. Use the explicit Issue URL/number from the request or triggering context; never
     derive it from the spec directory number. If no parent Issue is available, stop and ask for
     one before completing the spec because the Issue is the cross-agent handoff surface.
   - Do not persist the resolved path to `.specify/feature.json`. Repository
     handoff runs pass `SPECIFY_FEATURE_DIRECTORY` explicitly in every stage;
     a previous local session is not a source of truth.

   **IMPORTANT**:
   - You must only create one feature per `/speckit-specify` invocation
   - The spec directory name and the git branch name are independent — they may be the same but that is the user's choice
   - The spec directory and file are always created by this command, never by the hook

4. Load the resolved active `spec-template` file to understand required sections.

5. Load this repository's governance for its principles and constraints — [ARCHITECTURE.md](../../../ARCHITECTURE.md) for boundaries and dependency direction, [docs/design-docs/core-beliefs.md](../../../docs/design-docs/core-beliefs.md) for the judgement criteria, and [AGENTS.md](../../../AGENTS.md) for the working agreements. If `.specify/memory/constitution.md` exists, read it and let the principles it has actually ratified win; an unfilled template slot inside it is simply not a constraint, while a file that is absent, empty, or still nothing but placeholders carries none at all. This repository currently keeps no such file.

6. Follow this execution flow:
    1. Parse user description from arguments
       If empty: ERROR "No feature description provided"
    2. Extract key concepts from description
       Identify: actors, actions, data, constraints
    3. For unclear aspects, look for the answer before writing a question:
       - Resolve it from this repository's design docs and existing screens, the
         spec's own wording, or the form comparable current products have
         converged on. Write that answer into the spec and record it in
         Settled Without Asking with where it came from
       - Mark with [NEEDS CLARIFICATION: specific question] only when the answer
         changes what the user gets, no reference settles it (or two credible
         answers lead somewhere materially different), and getting it wrong
         would be expensive to undo. Then ask; do not pick one silently (Q-6)
       - Make an informed guess only for a detail that does not change what the
         user gets whichever way it goes, and record it in Settled Without Asking
       - An implementation difficulty is never the reason for a guess. If the
         obvious way to build something is hard, that is a question about the
         requirement, not a requirement (Q-7)
       - Carry at most 3 markers into one round of questions; the rest wait for
         the next round rather than being guessed
       - Prioritize by impact: scope > security/privacy > user experience > technical details
    4. Fill User Scenarios & Testing section
       If no clear user flow: ERROR "Cannot determine user scenarios"
    5. Generate Functional Requirements
       Each requirement must be testable
       Settle unspecified details from the references above and record them, with their source, in the Settled Without Asking section. Only a gap that no reference settles, and whose answer changes what the user gets, becomes a [NEEDS CLARIFICATION] marker (Q-6)
    6. Define Success Criteria
       Create measurable, technology-agnostic outcomes
       Include both quantitative metrics (time, performance, volume) and qualitative measures (user satisfaction, task completion)
       Each criterion must be verifiable without implementation details
    7. Identify Key Entities (if data involved)
    8. Return: SUCCESS (spec ready for planning)

7. Write the specification to SPEC_FILE using the template structure, replacing placeholders with concrete details derived from the feature description (arguments) while preserving section order and headings.

8. **Specification Quality Validation**: After writing the initial spec, review it in memory against these criteria. Do not create a validation artifact:

   - No implementation details (languages, frameworks, APIs)
   - Focused on user value and business needs
   - Written for non-technical stakeholders
   - All mandatory sections completed
   - No `[NEEDS CLARIFICATION]` markers remain — each one answered by the requester, never deleted by choosing an interpretation
   - Requirements are testable and unambiguous
   - Every functional requirement has clear acceptance criteria
   - User scenarios cover the primary flows
   - Success criteria are measurable and technology-agnostic
   - Acceptance scenarios and edge cases are defined
   - Scope, dependencies, and assumptions are clear
   - Feature requirements align with the measurable success outcomes

   Handle validation results as follows:

   - **If all criteria pass**: Proceed to the Mandatory Post-Execution Hooks section.

   - **If criteria fail (excluding [NEEDS CLARIFICATION])**:
     1. Update the spec to address each issue.
     2. Re-run validation until all criteria pass (max 3 iterations).
     3. If issues remain after 3 iterations, report them to the user.

      - **If [NEEDS CLARIFICATION] markers remain**:
        1. Extract all [NEEDS CLARIFICATION: ...] markers from the spec
        2. **BATCH**: Ask at most 3 per round, most critical first (by scope/security/UX impact). Markers that do not fit stay in the spec and are asked in the next round — never resolved by guessing (Q-6)
        3. For each clarification needed (max 3), present options to user in this format:

           ```markdown
           ## Question [N]: [Topic]

           **Context**: [Quote relevant spec section]

           **What we need to know**: [Specific question from NEEDS CLARIFICATION marker]

           **Suggested Answers**:

           | Option | Answer | Implications |
           |--------|--------|--------------|
           | A      | [First suggested answer] | [What this means for the feature] |
           | B      | [Second suggested answer] | [What this means for the feature] |
           | C      | [Third suggested answer] | [What this means for the feature] |
           | Custom | Provide your own answer | [Explain how to provide custom input] |

           **Your choice**: _[Wait for user response]_
           ```

        4. **CRITICAL - Table Formatting**: Ensure markdown tables are properly formatted:
           - Use consistent spacing with pipes aligned
           - Each cell should have spaces around content: `| Content |` not `|Content|`
           - Header separator must have at least 3 dashes: `|--------|`
           - Test that the table renders correctly in markdown preview
        5. Number questions sequentially (Q1, Q2, Q3 - max 3 total)
        6. Present all questions together before waiting for responses
        7. Wait for user to respond with their choices for all questions (e.g., "Q1: A, Q2: Custom - [details], Q3: B")
        8. Update the spec by replacing each [NEEDS CLARIFICATION] marker with the requester's selected or provided answer
        9. Re-run validation after all clarifications are resolved

## Mandatory Post-Execution Hooks

**You MUST complete this section before reporting completion to the user.**

Check if `.specify/extensions.yml` exists in the project root.
- If it does not exist, or no hooks are registered under `hooks.after_specify`, skip to the Completion Report.
- If it exists, read it and look for entries under the `hooks.after_specify` key.
- If the YAML cannot be parsed or is invalid, do not skip silently: tell the user that `.specify/extensions.yml` could not be read (include the parser error) and that no hooks were checked, including any mandatory (`optional: false`) hooks registered there, then continue to the Completion Report.
- Filter out hooks where `enabled` is explicitly `false`. Treat hooks without an `enabled` field as enabled by default.
- For each remaining hook, do **not** attempt to interpret or evaluate hook `condition` expressions:
  - If the hook has no `condition` field, or it is null/empty, treat the hook as executable
  - If the hook defines a non-empty `condition`, skip the hook and leave condition evaluation to the HookExecutor implementation
- When constructing command invocations from hook command names, replace dots (`.`) with hyphens (`-`). For example, `speckit.git.commit` → `/speckit-git-commit`.
- For each executable hook, output the following based on its `optional` flag:
  - **Mandatory hook** (`optional: false`) — **You MUST emit `EXECUTE_COMMAND:` for each mandatory hook**:
    ```
    ## Extension Hooks

    **Automatic Hook**: {extension}
    Executing: `/{command}`
    EXECUTE_COMMAND: {command}
    ```
    After emitting the block above you MUST actually invoke the hook and wait for it to finish before continuing. Run it the same way you would run the command yourself in this agent/session (the invocation may differ from the literal `{command}` id shown above, e.g. a skills-mode agent runs it as `/skill:speckit-...` or `$speckit-...`). Emitting the block alone does not run the hook.
  - **Optional hook** (`optional: true`):
    ```
    ## Extension Hooks

    **Optional Hook**: {extension}
    Command: `/{command}`
    Description: {description}

    Prompt: {prompt}
    To execute: `/{command}`
    ```

## Completion Report

Report completion to the user with:
- `SPECIFY_FEATURE_DIRECTORY` — the feature directory path
- `SPEC_FILE` — the spec file path
- Validation results summary
- Readiness for the next phase (`/speckit-clarify` or `/speckit-plan`)

**NOTE:** Branch creation is handled by the `before_specify` hook (git extension). Spec directory and file creation are always handled by this core command.

## Quick Guidelines

- Focus on **WHAT** users need and **WHY**.
- Avoid HOW to implement (no tech stack, APIs, code structure).
- Written for business stakeholders, not developers.
- DO NOT create any checklists that are embedded in the spec. That will be a separate command.

### Section Requirements

- **Mandatory sections**: Must be completed for every feature
- **Optional sections**: Include only when relevant to the feature
- When a section doesn't apply, remove it entirely (don't leave as "N/A")

### For AI Generation

When creating this spec from a user prompt:

1. **Answer it yourself first**: for each gap, look for the answer before
   considering a question. In order: this repository's design docs and existing
   screens, the spec's own wording, and the form that comparable current
   products have converged on. An answer found this way is written into the
   spec and recorded in Settled Without Asking — asking about something that already has a
   settled answer wastes the requester's attention and is itself a defect.
2. **Ask only what is genuinely open**: a gap becomes a
   `[NEEDS CLARIFICATION: specific question]` only when all three hold:
   - the answer changes what the user gets — the scope, the interaction, or the
     protection of their data; and
   - step 1 produced no answer, or produced two credible answers with
     materially different consequences; and
   - getting it wrong would be expensive to undo.
   Carry at most 3 into one round; the rest stay in the spec for the next round.
3. **Prioritize**: scope > security/privacy > user experience > technical details
4. **Document what you settled**: record each answer you took from a reference
   or a convention in the Settled Without Asking section, naming where it came from, so the
   requester can overturn it by reading rather than by being interrogated
5. **Think like a tester**: treat every vague requirement as a validation failure
6. **Keep implementation out of it**: a technical constraint is a question about
   the requirement, never a reason to narrow it (Q-7)

**Do not ask about these** — settle them from the references above and record
what you chose:

- How a common interaction looks and behaves when current products of the same
  kind have converged on one form. Follow that form; the reason to ask is a
  genuine fork, not the existence of a choice
- Visual layout, density, and component behavior already governed by this
  repository's design docs
- Performance targets, when the request implies no budget of its own
- Error handling: user-friendly messages with appropriate fallbacks
- Integration patterns: project-appropriate patterns (REST/GraphQL for web
  services, function calls for libraries, CLI args for tools, etc.)

**Worth asking, once the three conditions above hold**:

- Which use cases are in and which are out, when the request does not imply it
- A behavior where two credible designs lead somewhere materially different
- Data retention and the authentication method, when this product has no
  established position on them — both change how the user's data is protected

### Success Criteria Guidelines

Success criteria must be:

1. **Measurable**: Include specific metrics (time, percentage, count, rate)
2. **Technology-agnostic**: No mention of frameworks, languages, databases, or tools
3. **User-focused**: Describe outcomes from user/business perspective, not system internals
4. **Verifiable**: Can be tested/validated without knowing implementation details

**Good examples**:

- "Users can complete checkout in under 3 minutes"
- "System supports 10,000 concurrent users"
- "95% of searches return results in under 1 second"
- "Task completion rate improves by 40%"

**Bad examples** (implementation-focused):

- "API response time is under 200ms" (too technical, use "Users see results instantly")
- "Database can handle 1000 TPS" (implementation detail, use user-facing metric)
- "React components render efficiently" (framework-specific)
- "Redis cache hit rate above 80%" (technology-specific)

## Done When

- [ ] Specification written to `SPEC_FILE` and validated against the quality criteria
- [ ] Extension hooks dispatched or skipped according to the rules in Mandatory Post-Execution Hooks above
- [ ] Completion reported to user with feature directory, spec file path, and validation results
