---
name: "speckit-implement"
description: "Implement one explicitly requested unit of work using the feature specification and plan as context."
metadata:
  author: "github-spec-kit"
  source: "templates/commands/implement.md"
---

## Repository workflow

Implement only the one unit of work explicitly named by the user or supplied
Issue. When an Issue or PR is supplied, read and follow
`.agents/skills/issue-handoff/references/README.md` and
`.agents/skills/issue-handoff/references/implement.md` for repository handoff
behavior. A plain-text implementation request uses the current checkout and
does not require an Issue, parent Issue, feature directory, or PR.

Name the feature directory explicitly only when the request uses one.
## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Outline

1. For feature-directory work, the directory is given to you. List what it holds — `plan.md`, and any of `ui-design.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md` — and use that as the available-documents list. Run no script for this. For a plain-text implementation request, skip this step and use the current checkout and relevant repository files.

2. **Project Setup Verification**:
   - **REQUIRED**: Create/verify ignore files based on actual project setup:

   **Detection & Creation Logic**:
   - Check if the following command succeeds to determine if the repository is a git repo (create/verify .gitignore if so):

     ```sh
     git rev-parse --git-dir 2>/dev/null
     ```

   - Check if Dockerfile* exists or Docker in plan.md → create/verify .dockerignore
   - Check if .eslintrc* exists → create/verify .eslintignore
   - Check if eslint.config.* exists → ensure the config's `ignores` entries cover required patterns
   - Check if .prettierrc* exists → create/verify .prettierignore
   - Check if .npmrc or package.json exists → create/verify .npmignore (if publishing)
   - Check if terraform files (*.tf) exist → create/verify .terraformignore
   - Check if .helmignore needed (helm charts present) → create/verify .helmignore

   **If ignore file already exists**: Verify it contains essential patterns, append missing critical patterns only
   **If ignore file missing**: Create with full pattern set for detected technology

   **Common Patterns by Technology** (from plan.md tech stack):
   - **Node.js/JavaScript/TypeScript**: `node_modules/`, `dist/`, `build/`, `*.log`, `.env*`
   - **Python**: `__pycache__/`, `*.pyc`, `.venv/`, `venv/`, `dist/`, `*.egg-info/`
   - **Java**: `target/`, `*.class`, `*.jar`, `.gradle/`, `build/`
   - **C#/.NET**: `bin/`, `obj/`, `*.user`, `*.suo`, `packages/`
   - **Go**: `*.exe`, `*.test`, `vendor/`, `*.out`
   - **Ruby**: `.bundle/`, `log/`, `tmp/`, `*.gem`, `vendor/bundle/`
   - **PHP**: `vendor/`, `*.log`, `*.cache`, `*.env`
   - **Rust**: `target/`, `debug/`, `release/`, `*.rs.bk`, `*.rlib`, `*.prof*`, `.idea/`, `*.log`, `.env*`
   - **Kotlin**: `build/`, `out/`, `.gradle/`, `.idea/`, `*.class`, `*.jar`, `*.iml`, `*.log`, `.env*`
   - **C++**: `build/`, `bin/`, `obj/`, `out/`, `*.o`, `*.so`, `*.a`, `*.exe`, `*.dll`, `.idea/`, `*.log`, `.env*`
   - **C**: `build/`, `bin/`, `obj/`, `out/`, `*.o`, `*.a`, `*.so`, `*.exe`, `*.dll`, `autom4te.cache/`, `config.status`, `config.log`, `.idea/`, `*.log`, `.env*`
   - **Swift**: `.build/`, `DerivedData/`, `*.swiftpm/`, `Packages/`
   - **R**: `.Rproj.user/`, `.Rhistory`, `.RData`, `.Ruserdata`, `*.Rproj`, `packrat/`, `renv/`
   - **Universal**: `.DS_Store`, `Thumbs.db`, `*.tmp`, `*.swp`, `.vscode/`, `.idea/`

   **Tool-Specific Patterns**:
   - **Docker**: `node_modules/`, `.git/`, `Dockerfile*`, `.dockerignore`, `*.log*`, `.env*`, `coverage/`
   - **ESLint**: `node_modules/`, `dist/`, `build/`, `coverage/`, `*.min.js`
   - **Prettier**: `node_modules/`, `dist/`, `build/`, `coverage/`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`
   - **Terraform**: `.terraform/`, `*.tfstate*`, `*.tfvars`, `.terraform.lock.hcl`
   - **Kubernetes/k8s**: `*.secret.yaml`, `secrets/`, `.kube/`, `kubeconfig*`, `*.key`, `*.crt`

3. Resolve the requested work:
   - Use the user's request or supplied Issue as the scope boundary
   - Use the plan's implementation-work section and the supplied Issue as context
   - Read related work only to understand dependencies; do not implement it
   - If the requested work cannot be identified, ask the user instead of selecting all remaining tasks

4. Implement only the requested work and its necessary tests:
   - Respect dependencies without expanding the scope to unrelated work
   - Follow TDD when required by the specification or request
   - Include only setup, integration, and documentation changes necessary for this work
   - Run focused validation and any repository checks required by the change

5. Progress tracking and error handling:
   - Report progress for the requested work
   - Halt execution if a required step fails
   - Provide clear error messages with context for debugging
   - Suggest next steps if implementation cannot proceed

6. Completion validation:
   - Verify the requested work is complete
   - Check that the implementation matches the relevant specification and acceptance criteria
   - Validate that tests pass and coverage meets requirements
   - Confirm the implementation follows the technical plan

Note: Broader work described by the plan is context, not permission to implement
anything outside the explicit request.

## Completion Report

Report final status with summary of completed work.

## Done When

- [ ] Requested work completed without expanding into unrelated plan items
- [ ] Implementation validated against specification, plan, and test coverage
- [ ] Completion reported to user with summary of completed work
