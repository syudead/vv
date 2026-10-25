import { FolderSearch, Layers, Plus } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";

import { createCredit } from "../../api/credits";
import { currentTags, revalidateTags, subscribeTags, type Tag } from "../../api/tags";
import { applyVideoEdits, type VideoEditSummary } from "../../api/videoEdits";
import {
  type WorkDateKind,
  workOrganizationRoles,
  workPersonRoles,
} from "../../api/workInfo";
import { creditDisplayName } from "../../credits/displayName";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import CreditPicker from "../../player/detailsEditor/CreditPicker";
import { organizationRole, personRole } from "../../player/detailsEditor/CreditsPanel";
import { type Chip, isNewChip } from "../../player/detailsEditor/draft";
import { ConfirmDialog } from "../../ui/patterns/confirm-dialog";
import { FormPageDialog, type FormPagePanel } from "../../ui/patterns/form-page";
import { ErrorState } from "../../ui/patterns/error-state";
import { Button } from "../../ui/shadcn/button";
import { Skeleton } from "../../ui/shadcn/skeleton";
import TagCommand from "../../ui/TagCommand";
import { useToast } from "../../ui/Toast";
import { VideoThumbnail, VideoThumbnailImage } from "../../ui/VideoThumbnail";
import { type RemotePartial, remotePartial, videoTitleIn } from "../RemotePartialDialog";
import { editTargetsOf, type SelectionTarget } from "../selection";
import { buildAddOptions } from "../tagChoices";
import DateField from "./DateField";
import {
  type BulkDraft,
  buildChanges,
  type ChangeLine,
  changeLines,
  type CreditKind,
  creditKey,
  cycleValue,
  emptyDraft,
  type FieldId,
  fieldOf,
  newCredits,
  pickValue,
  presentCredits,
  presentTags,
  type PresentValue,
  setDate,
  setPublic,
  tagKey,
  unpickValue,
} from "./draft";
import { useEditSummary } from "./useEditSummary";
import ValueField from "./ValueField";
import VisibilityField from "./VisibilityField";

const dateKinds: readonly WorkDateKind[] = ["release", "recorded", "published"];

interface RoleEntry {
  id: FieldId;
  kind: CreditKind;
  role: string;
  label: UiText;
  add: UiText;
}

const roleEntries: RoleEntry[] = [
  ...workPersonRoles.map((role) => ({
    id: `person:${role}` as const,
    kind: "person" as const,
    role,
    ...personRole(role),
  })),
  ...workOrganizationRoles.map((role) => ({
    id: `organization:${role}` as const,
    kind: "organization" as const,
    role,
    ...organizationRole(role),
  })),
];

function presentMap(summary: VideoEditSummary): Map<FieldId, PresentValue[]> {
  const map = new Map<FieldId, PresentValue[]>([["tags", presentTags(summary)]]);
  for (const entry of roleEntries) {
    map.set(entry.id, presentCredits(summary, entry.kind, entry.role));
  }
  return map;
}

function useAllTags(): Tag[] {
  const [tags, setTags] = useState<Tag[]>(currentTags() ?? []);
  useEffect(() => {
    const unsubscribe = subscribeTags(setTags);
    revalidateTags().catch(() => undefined);
    return unsubscribe;
  }, []);
  return tags;
}

/**
 * The bulk editor over the selected cards: the stage lists the targets and leaves out the ones
 * pressed, the aside shows what the remaining videos carry and what Apply will change.
 * Mount only while open; `onApplied` receives the removed tag IDs so lists filtered by them reload.
 */
export default function BulkEditor({
  targets,
  onClose,
  onApplied,
}: {
  targets: readonly SelectionTarget[];
  onClose: () => void;
  onApplied: (removedTagIds: readonly number[], partial: RemotePartial | null) => void;
}) {
  const b = t.library.bulkEdit;
  const toast = useToast();
  const [excluded, setExcluded] = useState<ReadonlySet<string>>(new Set());
  const [draft, setDraft] = useState<BulkDraft>(emptyDraft);
  const [attempt, setAttempt] = useState(0);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const created = useRef(new Map<string, number>());

  const included = targets.filter((target) => !excluded.has(target.key));
  const includedKey = included.map((target) => target.key).join(",");
  const editTargets = editTargetsOf(included);
  const state = useEditSummary(editTargets, includedKey, attempt);
  const summary =
    state.kind === "ready"
      ? state.summary
      : state.kind === "loading"
        ? state.previous
        : undefined;
  const present = useMemo(
    () =>
      summary === undefined ? new Map<FieldId, PresentValue[]>() : presentMap(summary),
    [summary],
  );
  const lines = summary === undefined ? [] : changeLines(draft, summary, present);
  const ready = state.kind === "ready" && included.length > 0;

  function requestClose() {
    if (pending) return;
    if (lines.length > 0) setConfirmOpen(true);
    else onClose();
  }

  async function send(current: VideoEditSummary) {
    for (const entry of newCredits(draft)) {
      if (created.current.has(entry.key)) continue;
      const credit = await createCredit(entry.kind, { name: entry.name });
      created.current.set(entry.key, credit.id);
    }
    const { changes, removedTags } = buildChanges(
      draft,
      current,
      present,
      created.current,
    );
    const result = await applyVideoEdits(editTargets, changes, removedTags);
    return {
      applied: result.applied,
      removedTagIds: removedTags.map((tag) => tag.id),
      partial: remotePartial(result.remote, videoTitleIn(included), "changed"),
    };
  }

  function apply() {
    if (!ready || pending || lines.length === 0) return;
    setPending(true);
    setFailure(null);
    send(state.summary).then(
      ({ applied, removedTagIds, partial }) => {
        // A partial answer names the peer's failures in a dialog instead of a toast.
        if (partial === null) toast(b.applied(applied));
        onApplied(removedTagIds, partial);
        onClose();
      },
      (error: unknown) => {
        setFailure(b.failed(errorText(error)));
        setPending(false);
      },
    );
  }

  const total = summary?.total ?? 0;
  const panels =
    summary === undefined || included.length === 0
      ? loadingPanels(included.length === 0 && summary !== undefined)
      : fieldPanels({ summary, present, draft, disabled: pending, onChange: setDraft });
  return (
    <>
      <FormPageDialog
        open
        onClose={requestClose}
        title={b.title}
        detail={summary === undefined ? b.counting : b.detail(total)}
        actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              aria-disabled={pending || undefined}
              onClick={requestClose}
            >
              {t.player.detailsEditor.cancel}
            </Button>
            <Button
              size="sm"
              aria-disabled={!ready || lines.length === 0 || pending || undefined}
              aria-busy={pending || undefined}
              onClick={apply}
            >
              {pending ? b.applying : b.apply(total)}
            </Button>
          </>
        }
        notice={
          failure !== null && (
            <p role="alert" className="text-sm text-destructive">
              {failure}
            </p>
          )
        }
        asideLabel={b.targets}
        aside={
          <>
            <TargetList
              targets={targets}
              excluded={excluded}
              disabled={pending}
              onToggle={(key) =>
                setExcluded((current) => {
                  const next = new Set(current);
                  if (!next.delete(key)) next.add(key);
                  return next;
                })
              }
              onIncludeAll={() => setExcluded(new Set())}
            />
            <ChangeList lines={lines} />
          </>
        }
        panels={panels}
        status={
          state.kind === "failed" ? (
            <ErrorState
              title={b.summaryFailed(state.reason)}
              retryLabel={t.common.retry}
              onRetry={() => setAttempt((value) => value + 1)}
            />
          ) : undefined
        }
      />
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={b.discardTitle}
        description={b.discardText}
        actionLabel={t.player.detailsEditor.discard}
        cancelLabel={t.player.detailsEditor.keepEditing}
        onConfirm={onClose}
      />
    </>
  );
}

function loadingPanels(allExcluded: boolean): FormPagePanel[] {
  const b = t.library.bulkEdit;
  const content = allExcluded ? (
    <p className="text-sm text-muted-foreground">{b.allExcluded}</p>
  ) : (
    <div role="status" aria-label={b.loading} className="flex flex-col gap-4">
      {[0, 1, 2].map((index) => (
        <Skeleton key={index} className="h-12 w-full" />
      ))}
    </div>
  );
  return [
    { key: "tags-people", legend: b.tagsAndPeople, content },
    { key: "others", legend: b.organizationsDatesVisibility, content: null },
  ];
}

/** Rows shown before `Show all`, so a large selection does not push the change list away. */
const targetRowsShown = 12;

function TargetList({
  targets,
  excluded,
  disabled,
  onToggle,
  onIncludeAll,
}: {
  targets: readonly SelectionTarget[];
  excluded: ReadonlySet<string>;
  disabled: boolean;
  onToggle: (key: string) => void;
  onIncludeAll: () => void;
}) {
  const b = t.library.bulkEdit;
  const [all, setAll] = useState(false);
  const shown = all ? targets : targets.slice(0, targetRowsShown);
  return (
    <section className="flex flex-col gap-2">
      <div className="flex min-h-8 items-center justify-between gap-2">
        <h2 className="font-medium">{b.targets}</h2>
        {excluded.size > 0 && (
          <Button variant="ghost" size="sm" disabled={disabled} onClick={onIncludeAll}>
            {b.includeAll(excluded.size)}
          </Button>
        )}
      </div>
      <p className="text-xs text-muted-foreground">{b.stageHint}</p>
      <ul aria-label={b.targets} className="flex flex-col gap-1">
        {shown.map((target) => (
          <TargetRow
            key={target.key}
            target={target}
            included={!excluded.has(target.key)}
            disabled={disabled}
            onToggle={() => onToggle(target.key)}
          />
        ))}
      </ul>
      {targets.length > shown.length && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          onClick={() => setAll(true)}
        >
          {b.showAll(targets.length)}
        </Button>
      )}
    </section>
  );
}

function TargetRow({
  target,
  included,
  disabled,
  onToggle,
}: {
  target: SelectionTarget;
  included: boolean;
  disabled: boolean;
  onToggle: () => void;
}) {
  const b = t.library.bulkEdit;
  const Icon = target.kind === "smartFolder" ? FolderSearch : Layers;
  return (
    <li className="min-w-0">
      <Button
        variant="ghost"
        aria-pressed={included}
        aria-label={b.tileLabel(target.label, included)}
        aria-disabled={disabled || undefined}
        onClick={() => {
          if (!disabled) onToggle();
        }}
        className="h-auto w-full justify-start gap-2 p-1 font-normal"
      >
        <VideoThumbnail className={cn("w-16 shrink-0", !included && "opacity-40")}>
          {target.thumbnailUrl !== undefined && (
            <VideoThumbnailImage src={target.thumbnailUrl} />
          )}
        </VideoThumbnail>
        <span className="flex min-w-0 flex-1 flex-col items-start">
          <span
            className={cn(
              "w-full truncate text-left text-sm",
              !included && "text-muted-foreground line-through",
            )}
          >
            {target.label}
          </span>
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            {target.kind !== "video" && <Icon className="size-3" aria-hidden="true" />}
            {!included
              ? b.excluded
              : target.kind === "group"
                ? b.groupMark(target.videoIds.length)
                : target.kind === "smartFolder"
                  ? b.smartFolderMark
                  : null}
          </span>
        </span>
      </Button>
    </li>
  );
}

function ChangeList({ lines }: { lines: readonly ChangeLine[] }) {
  const b = t.library.bulkEdit;
  return (
    <section className="flex flex-col gap-2 border-t border-border pt-3">
      <h2 className="font-medium">{b.changes}</h2>
      {lines.length === 0 ? (
        <p className="text-xs text-muted-foreground">{b.noChanges}</p>
      ) : (
        <ul aria-label={b.changes} className="flex flex-col gap-1 text-xs">
          {lines.map((line, index) => (
            <li key={`${line.field}-${String(index)}`} className="wrap-anywhere">
              {changeText(line)}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

interface FieldProps {
  summary: VideoEditSummary;
  present: ReadonlyMap<FieldId, readonly PresentValue[]>;
  draft: BulkDraft;
  disabled: boolean;
  onChange: (change: (draft: BulkDraft) => BulkDraft) => void;
}

/** Tags and people fill the first panel, organizations, dates and visibility the second. */
function fieldPanels({
  summary,
  present,
  draft,
  disabled,
  onChange,
}: FieldProps): FormPagePanel[] {
  const b = t.library.bulkEdit;
  const valueField = (id: FieldId, legend: UiText, picker: ReactNode) => (
    <ValueField
      key={id}
      legend={legend}
      present={present.get(id) ?? []}
      total={summary.total}
      field={fieldOf(draft, id)}
      disabled={disabled}
      onCycle={(value) =>
        onChange((current) => cycleValue(current, id, value, summary.total))
      }
      onUnpick={(key) => onChange((current) => unpickValue(current, id, key))}
      picker={picker}
    />
  );
  const creditField = (entry: RoleEntry) =>
    valueField(
      entry.id,
      entry.label,
      <CreditPicker
        kind={entry.kind}
        offerUnnamed={entry.id === "person:performer"}
        label={entry.add}
        chips={pickedChips(draft, entry.id)}
        unsaved={unsavedNames(draft, entry.kind)}
        disabled={disabled}
        onAdd={(chip) =>
          onChange((current) =>
            pickValue(current, entry.id, pickedCredit(chip), present.get(entry.id) ?? []),
          )
        }
      />,
    );
  return [
    {
      key: "tags-people",
      legend: b.tagsAndPeople,
      content: (
        <>
          <p className="text-xs text-muted-foreground">{b.howTo}</p>
          {valueField(
            "tags",
            b.tags,
            <TagPicker
              disabled={disabled}
              onPick={(value) =>
                onChange((current) =>
                  pickValue(current, "tags", value, present.get("tags") ?? []),
                )
              }
            />,
          )}
          {roleEntries.filter((entry) => entry.kind === "person").map(creditField)}
        </>
      ),
    },
    {
      key: "others",
      legend: b.organizationsDatesVisibility,
      content: (
        <>
          {roleEntries.filter((entry) => entry.kind === "organization").map(creditField)}
          {dateKinds.map((kind) => (
            <DateField
              key={kind}
              kind={kind}
              total={summary.total}
              current={summary.dates.find((item) => item.kind === kind)}
              draft={draft.dates[kind]}
              disabled={disabled}
              onChange={(date) => onChange((current) => setDate(current, kind, date))}
            />
          ))}
          <VisibilityField
            total={summary.total}
            publicCount={summary.public}
            value={draft.public}
            disabled={disabled}
            onChange={(value) => onChange((current) => setPublic(current, value))}
          />
        </>
      ),
    },
  ];
}

function pickedCredit(chip: Chip) {
  return isNewChip(chip)
    ? { key: `new:${chip.newKey}`, label: chip.name, newName: chip.name }
    : {
        key: creditKey(chip.id),
        label: creditDisplayName(chip),
        id: chip.id,
      };
}

function pickedChips(draft: BulkDraft, id: FieldId): Chip[] {
  return fieldOf(draft, id).picked.map((value) =>
    "id" in value
      ? { key: value.key, id: value.id, name: value.label }
      : { key: value.key, newKey: value.key.slice("new:".length), name: value.label },
  );
}

/** New names typed in one role are offered in the other roles of the same kind, so one credit is created. */
function unsavedNames(draft: BulkDraft, kind: CreditKind) {
  const seen = new Set<string>();
  const names: { newKey: string; name: string }[] = [];
  for (const entry of roleEntries) {
    if (entry.kind !== kind) continue;
    for (const value of fieldOf(draft, entry.id).picked) {
      if (!("newName" in value) || seen.has(value.key)) continue;
      seen.add(value.key);
      names.push({ newKey: value.key.slice("new:".length), name: value.newName });
    }
  }
  return names;
}

function TagPicker({
  disabled,
  onPick,
}: {
  disabled: boolean;
  onPick: (
    value:
      | { key: string; label: string; id: number }
      | { key: string; label: string; newName: string },
  ) => void;
}) {
  const allTags = useAllTags();
  const [value, setValue] = useState("");
  const { options, exactOption } = buildAddOptions(allTags, value);
  const trimmed = value.trim();
  const createLabel =
    exactOption === null && trimmed !== "" ? (
      <span className="flex min-w-0 items-center gap-2">
        <Plus className="text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{t.library.selection.create(trimmed)}</span>
      </span>
    ) : null;
  return (
    <TagCommand
      layout="dropdown"
      label={t.library.bulkEdit.addTag}
      value={value}
      onValueChange={setValue}
      choices={options}
      exactChoice={exactOption}
      onSelect={(option) => {
        setValue("");
        const id = Number(option.id);
        onPick({ key: tagKey(id), label: option.label, id });
      }}
      createLabel={createLabel}
      onCreate={(spelling) => {
        setValue("");
        onPick({ key: `n:${spelling}`, label: spelling, newName: spelling });
      }}
      icon={<Plus className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />}
      busy={false}
      locked={disabled}
    />
  );
}

function fieldLabel(field: ChangeLine["field"]): UiText {
  const b = t.library.bulkEdit;
  if (field === "tags") return b.tags;
  if (field === "public") return b.visibility;
  if (field === "release" || field === "recorded" || field === "published")
    return b.dateLabel(field);
  return roleEntries.find((entry) => entry.id === field)?.label ?? b.tags;
}

function changeText(line: ChangeLine): UiText {
  const b = t.library.bulkEdit;
  const field = fieldLabel(line.field);
  switch (line.action) {
    case "add":
      return b.changeAdd(field, line.label, line.videos);
    case "remove":
      return b.changeRemove(field, line.label, line.videos);
    case "set":
      return b.changeSet(field, line.label, line.videos);
    case "clear":
      return b.changeClear(field, line.videos);
    case "public":
      return b.changePublic(line.videos);
    case "private":
      return b.changePrivate(line.videos);
  }
}
