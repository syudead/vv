# Architecture

This repository is an initial harness scaffold and does not yet contain an
application. Document the system's major components, boundaries, dependency
direction, and runtime topology here as implementation is introduced.

## Principles

- Make module boundaries explicit and mechanically enforceable where possible.
- Keep dependencies directed from product-facing layers toward stable domain
  interfaces.
- Capture consequential design decisions in `docs/design-docs/`.
- Generate volatile reference material into `docs/generated/`.
