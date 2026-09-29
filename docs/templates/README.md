# Document templates

Each file here fixes the sections of one document type. Copy the template,
replace every `{{...}}` placeholder, and delete the sections that have nothing to
say. The rules behind the shapes are in
[writing-style.md](../design-docs/writing-style.md).

| Template | Produces | Written by |
| --- | --- | --- |
| [design-doc.md](design-doc.md) | `docs/design-docs/<topic>.md` | Any change that makes a lasting technical decision |
| [how-to.md](how-to.md) | `docs/how-to/<task>.md` | Any change that adds an operator or developer procedure |
| [research.md](research.md) | `specs/<feature>/research.md` | The `plan` stage |
| [data-model.md](data-model.md) | `specs/<feature>/data-model.md` | The `plan` stage |
| [contract.md](contract.md) | `specs/<feature>/contracts/<name>.md` | The `plan` stage |
| [quickstart.md](quickstart.md) | `specs/<feature>/quickstart.md` | The `plan` stage |
| [ui-design.md](ui-design.md) | `specs/<feature>/ui-design.md` | The `design` stage |

The Plan template lives with the skill that fills it:
[plan-template.md](../../.agents/skills/sdd-plan/assets/plan-template.md).

`{{...}}` placeholders are checked: `task check-docs` fails when one reaches a
file under `specs/` or `docs/` outside this directory.

Relative links inside a template are written for the file's destination, not
for this directory, so the link checks skip `docs/templates/` and the
documentation site does not publish it.
