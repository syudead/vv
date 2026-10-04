# Vendored shadcn skill

This directory is the official shadcn skill from
[shadcn-ui/ui `skills/shadcn`](https://github.com/shadcn-ui/ui/tree/295a1f114a138f23b5dfee0e0c6812394dfeb90c/skills/shadcn)
at commit `295a1f114a138f23b5dfee0e0c6812394dfeb90c`, without its `evals/`.

It has one local change: every `npx shadcn@latest` (and the `pnpm dlx` and
`bunx` forms) runs `npm --prefix web exec shadcn --` instead, so an agent uses
the CLI version pinned in `web/package.json`
([038 research R-7](../../../specs/038-design-system/research.md#r-7-agents-reach-the-registry-through-agentsmd-a-vendored-skill-and-an-mcp-server)).
`web/src/theme/vendoredSkill.test.ts` fails when `shadcn@` appears here again.
How to update it is in
[dependency-updates.md](../../../docs/how-to/dependency-updates.md).
