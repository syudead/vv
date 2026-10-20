import { type FormEvent, Fragment, type ReactNode, useId, useState } from "react";

import { type CreditKind, createCredit } from "../api/credits";
import {
  editVideoCredits,
  type VideoCreditEdit,
  workOrganizationRoles,
  workPersonRoles,
} from "../api/workInfo";
import { errorText, t, type UiText } from "../i18n";
import CreditPicker, { type UnsavedName } from "../player/detailsEditor/CreditPicker";
import {
  CreditChip,
  organizationRole,
  personRole,
} from "../player/detailsEditor/CreditsPanel";
import { type Chip, isNewChip } from "../player/detailsEditor/draft";
import { FormDialog } from "../ui/patterns/form-dialog";
import { FieldError, FieldLegend, FieldSet, FieldTitle } from "../ui/shadcn/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import { useToast } from "../ui/Toast";

type Action = VideoCreditEdit["action"];

interface RoleEntry {
  kind: CreditKind;
  role: string;
  label: UiText;
  add: UiText;
}

interface RoleDraft {
  action: Action;
  chips: Chip[];
}

const personEntries: RoleEntry[] = workPersonRoles.map((role) => ({
  kind: "person",
  role,
  ...personRole(role),
}));
const organizationEntries: RoleEntry[] = workOrganizationRoles.map((role) => ({
  kind: "organization",
  role,
  ...organizationRole(role),
}));

function entryKey(entry: RoleEntry): string {
  return `${entry.kind}:${entry.role}`;
}

const emptyDraft: RoleDraft = { action: "add", chips: [] };

/** A role is sent when it names credits, or when Replace is chosen to clear it. */
function isSent(draft: RoleDraft): boolean {
  return draft.chips.length > 0 || draft.action === "replace";
}

/** Create the typed names once per kind, then send every role's IDs in one request. */
async function sendEdits(
  drafts: ReadonlyMap<string, RoleDraft>,
  send: (edits: VideoCreditEdit[]) => Promise<number>,
): Promise<number> {
  const created = new Map<string, number>();
  const edits: VideoCreditEdit[] = [];
  for (const entry of [...personEntries, ...organizationEntries]) {
    const draft = drafts.get(entryKey(entry));
    if (draft === undefined || !isSent(draft)) continue;
    const ids: number[] = [];
    for (const chip of draft.chips) {
      if (!isNewChip(chip)) {
        ids.push(chip.id);
        continue;
      }
      const key = `${entry.kind}:${chip.newKey}`;
      let id = created.get(key);
      if (id === undefined) {
        id = (await createCredit(entry.kind, { name: chip.name })).id;
        created.set(key, id);
      }
      ids.push(id);
    }
    edits.push({ kind: entry.kind, role: entry.role, action: draft.action, ids });
  }
  return send(edits);
}

/**
 * Mount only while open. Each role is added to, removed from or replaced on every selected video;
 * the server resolves smart folders to their current members.
 */
export default function EditCreditsDialog({
  count,
  selectedIds,
  smartFolderIds,
  onClose,
}: {
  count: number;
  selectedIds: readonly number[];
  smartFolderIds: readonly number[];
  onClose: () => void;
}) {
  const words = t.library.selection.creditsDialog;
  const toast = useToast();
  const failureId = useId();
  const [drafts, setDrafts] = useState<ReadonlyMap<string, RoleDraft>>(new Map());
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const ready = [...drafts.values()].some(isSent);

  function update(entry: RoleEntry, change: (draft: RoleDraft) => RoleDraft) {
    setDrafts((current) => {
      const next = new Map(current);
      next.set(entryKey(entry), change(current.get(entryKey(entry)) ?? emptyDraft));
      return next;
    });
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || !ready) return;
    setPending(true);
    setFailure(null);
    sendEdits(drafts, (edits) =>
      editVideoCredits(selectedIds, edits, { smartFolderIds }).then(
        (result) => result.applied,
      ),
    ).then(
      (applied) => {
        toast(t.library.selection.creditsEdited(applied));
        onClose();
      },
      (error: unknown) => {
        setFailure(words.failed(errorText(error)));
        setPending(false);
      },
    );
  }

  const rows = (entries: RoleEntry[]) =>
    entries.map((entry) => (
      <RoleRow
        key={entryKey(entry)}
        entry={entry}
        draft={drafts.get(entryKey(entry)) ?? emptyDraft}
        unsaved={unsavedNames(drafts, entries)}
        pending={pending}
        onChange={(change) => update(entry, change)}
      />
    ));

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={words.title}
      description={words.description(count)}
      onSubmit={submit}
      submitLabel={words.submit}
      cancelLabel={t.player.detailsEditor.cancel}
      size="wide"
      pending={pending}
      submitDisabled={!ready}
    >
      <RoleGroup legend={t.player.detailsEditor.people}>{rows(personEntries)}</RoleGroup>
      <RoleGroup legend={t.player.detailsEditor.organizations}>
        {rows(organizationEntries)}
      </RoleGroup>
      {failure !== null ? (
        <FieldError id={failureId}>{failure}</FieldError>
      ) : (
        !ready && <p className="text-xs text-muted-foreground">{words.nothingToApply}</p>
      )}
    </FormDialog>
  );
}

function RoleGroup({ legend, children }: { legend: UiText; children: ReactNode }) {
  return (
    <FieldSet className="gap-0">
      <FieldLegend variant="label">{legend}</FieldLegend>
      <div className="grid grid-cols-term items-start gap-x-4 gap-y-3">{children}</div>
    </FieldSet>
  );
}

/** Names typed in one role are offered in the other roles of the same kind, so one credit is created. */
function unsavedNames(
  drafts: ReadonlyMap<string, RoleDraft>,
  entries: readonly RoleEntry[],
): UnsavedName[] {
  const seen = new Set<string>();
  return entries.flatMap((entry) =>
    (drafts.get(entryKey(entry))?.chips ?? []).flatMap((chip) => {
      if (!isNewChip(chip) || seen.has(chip.newKey)) return [];
      seen.add(chip.newKey);
      return [{ newKey: chip.newKey, name: chip.name }];
    }),
  );
}

function RoleRow({
  entry,
  draft,
  unsaved,
  pending,
  onChange,
}: {
  entry: RoleEntry;
  draft: RoleDraft;
  unsaved: readonly UnsavedName[];
  pending: boolean;
  onChange: (change: (draft: RoleDraft) => RoleDraft) => void;
}) {
  const words = t.library.selection.creditsDialog;
  const labelId = useId();
  const clears = draft.action === "replace" && draft.chips.length === 0;
  return (
    <Fragment>
      <FieldTitle id={labelId} className="pt-1.5 whitespace-nowrap">
        {entry.label}
      </FieldTitle>
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <Select
            value={draft.action}
            disabled={pending}
            onValueChange={(action) =>
              onChange((current) => ({ ...current, action: action as Action }))
            }
          >
            <SelectTrigger size="sm" aria-label={words.modeLabel(entry.label)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="add">{words.add}</SelectItem>
              <SelectItem value="remove">{words.remove}</SelectItem>
              <SelectItem value="replace">{words.replace}</SelectItem>
            </SelectContent>
          </Select>
          <ul
            aria-labelledby={labelId}
            className="flex min-w-0 flex-wrap items-center gap-1.5"
          >
            {draft.chips.map((chip) => (
              <CreditChip
                key={chip.key}
                chip={chip}
                saving={pending}
                problem={null}
                onRemove={() => {
                  if (pending) return;
                  onChange((current) => ({
                    ...current,
                    chips: current.chips.filter((item) => item.key !== chip.key),
                  }));
                }}
              />
            ))}
            <li className="max-w-full min-w-0">
              <CreditPicker
                kind={entry.kind}
                label={entry.add}
                chips={draft.chips}
                unsaved={unsaved}
                disabled={pending}
                onAdd={(chip) =>
                  onChange((current) => ({ ...current, chips: [...current.chips, chip] }))
                }
              />
            </li>
          </ul>
        </div>
        {clears && (
          <p className="text-xs text-muted-foreground">{words.clears(entry.label)}</p>
        )}
      </div>
    </Fragment>
  );
}
