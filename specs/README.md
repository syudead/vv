# Temporary SDD artifacts

An active SDD feature keeps its Plan and supporting artifacts here on its
feature branch. The parent GitHub Issue owns the requirements. These files
are working inputs for that feature, not references for later implementation
or investigation.

Before integration, record lasting decisions in `docs/design-docs/`, update
the permanent references, and delete the feature directory after validation. Git history retains the intermediate work.
Do not move the whole artifact set into design docs or a repository archive.

The current system is described by design docs, code, API schemas and tests.
Permanent documents and code never cite a feature directory here.
