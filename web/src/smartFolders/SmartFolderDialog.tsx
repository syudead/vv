import { CircleAlert } from "lucide-react";
import { type FormEvent, useEffect, useId, useRef, useState } from "react";

import { RequestFailed, type VideoSort } from "../api/client";
import {
  changeSmartFolder,
  createSmartFolder,
  listSmartFolders,
  type SmartFolderChange,
  type SmartFolderSummary,
  type SmartRule,
  type SmartRuleRef,
} from "../api/smartFolders";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { playlistName } from "../playlists/playlistName";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Field, FieldDescription, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/shadcn/select";
import { Spinner } from "../ui/shadcn/spinner";
import { normalizeQuery, sortKinds, sortKindLabel } from "../videoList/listCriteria";
import ConditionsFields from "./ConditionsFields";
import { cleanRule, hasRuleMember, parentChoices, ruleKey, validRule } from "./smartRule";

export type SmartFolderDialogTarget =
  | {
      kind: "create";
      rule: SmartRule;
      sort: VideoSort;
      /** Random sort was replaced with newest-added; show the accompanying note. */
      randomReplaced?: boolean;
      parent?: { id: number; name: string };
    }
  | {
      kind: "edit";
      folder: { id: number; name: string; parentId?: number };
      rule: SmartRule;
      sort: VideoSort;
      missing: readonly SmartRuleRef[];
    };

/** Cap indentation at four levels. */
const indentClasses = ["pl-2", "pl-6", "pl-10", "pl-14", "pl-18"] as const;

function indentClass(depth: number): string {
  return indentClasses[Math.min(depth, indentClasses.length - 1)] ?? "pl-2";
}

/** Use a non-positive sentinel because real IDs are positive. */
const noParentValue = "none";

function sortChoices(): { value: VideoSort; label: UiText }[] {
  const words: Partial<Record<string, Record<"asc" | "desc", UiText>>> =
    t.list.sort.wording;
  return sortKinds.flatMap((info) =>
    (["desc", "asc"] as const).flatMap((direction) => {
      const value = info[direction];
      if (value === undefined) return [];
      const wording =
        words[info.kind]?.[direction] ??
        (direction === "asc"
          ? t.smartFolders.dialog.ascending
          : t.smartFolders.dialog.descending);
      return [
        {
          value,
          label: t.smartFolders.dialog.sortOption(sortKindLabel(info.kind), wording),
        },
      ];
    }),
  );
}

interface FieldErrors {
  name: boolean;
  parent: UiText | null;
  conditions: UiText | null;
}

const noErrors: FieldErrors = { name: false, parent: null, conditions: null };

function failureErrors(error: unknown): FieldErrors | null {
  if (!(error instanceof RequestFailed)) return null;
  if (error.status === 400 && error.code === "invalid_smart_rule") {
    return { ...noErrors, conditions: t.errors.code.invalid_smart_rule };
  }
  if (error.status === 400) return { ...noErrors, name: true };
  if (error.status === 404 && error.code === "smart_folder_not_found") {
    return { ...noErrors, parent: t.errors.code.smart_folder_not_found };
  }
  if (error.status === 409 && error.code === "smart_folder_cycle") {
    return { ...noErrors, parent: t.errors.code.smart_folder_cycle };
  }
  return null;
}

/** Reload parent candidates after a selected parent returns 404. */
function useParentFolders() {
  const [folders, setFolders] = useState<SmartFolderSummary[] | undefined>();
  const [failure, setFailure] = useState<UiText | null>(null);
  const [round, setRound] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    listSmartFolders(controller.signal).then(
      (loaded) => {
        setFolders(loaded);
        setFailure(null);
      },
      (error: unknown) => {
        if (controller.signal.aborted) return;
        setFailure(t.smartFolders.listFailed(errorText(error)));
      },
    );
    return () => controller.abort();
  }, [round]);
  return { folders, failure, reload: () => setRound((value) => value + 1) };
}

function dialogTitle(target: SmartFolderDialogTarget): UiText {
  if (target.kind === "edit") return t.smartFolders.dialog.editTitle;
  return target.parent === undefined
    ? t.smartFolders.dialog.newTitle
    : t.smartFolders.dialog.newChildTitle(target.parent.name);
}

function initialParent(target: SmartFolderDialogTarget): number | undefined {
  return target.kind === "edit" ? target.folder.parentId : target.parent?.id;
}

/** Compare normalized submitted rules so equivalent edits do not produce patches. */
function editChange(
  target: Extract<SmartFolderDialogTarget, { kind: "edit" }>,
  draft: { name: string; parentId: number | undefined; sort: VideoSort; rule: SmartRule },
): SmartFolderChange {
  const change: SmartFolderChange = {};
  if (draft.name !== target.folder.name) change.name = draft.name;
  if (draft.parentId !== target.folder.parentId) change.parentId = draft.parentId ?? null;
  if (draft.sort !== target.sort) change.sort = draft.sort;
  if (ruleKey(draft.rule) !== ruleKey(target.rule)) change.rule = draft.rule;
  return change;
}

/** Normalize search only when edited; preserve untouched contract-valid queries up to 500 characters. */
function sentRule(rule: SmartRule, original: SmartRule): SmartRule {
  const q = rule.q === original.q ? rule.q : normalizeQuery(rule.q ?? "");
  return cleanRule({ ...rule, q });
}

function submitWords(target: SmartFolderDialogTarget): { idle: UiText; busy: UiText } {
  return target.kind === "create"
    ? { idle: t.smartFolders.dialog.submit, busy: t.smartFolders.dialog.submitting }
    : { idle: t.smartFolders.dialog.saveSubmit, busy: t.smartFolders.dialog.saving };
}

/** Exclude this folder and descendants; prefer field errors over parent-list loading errors. */
function ParentField({
  target,
  parent,
  parents,
  editing,
  error,
  onChange,
}: {
  target: SmartFolderDialogTarget;
  parent: number | undefined;
  parents: { folders: SmartFolderSummary[] | undefined; failure: UiText | null };
  editing: number | undefined;
  error: UiText | null;
  onChange: (parent: number | undefined) => void;
}) {
  const parentId = useId();
  const choices = parentChoices(parents.folders ?? [], editing);
  return (
    <Field data-invalid={error !== null || undefined}>
      <FieldLabel htmlFor={parentId}>{t.smartFolders.dialog.parent}</FieldLabel>
      <Select
        value={parent === undefined ? noParentValue : String(parent)}
        onValueChange={(next) =>
          onChange(next === noParentValue ? undefined : Number(next))
        }
      >
        <SelectTrigger id={parentId} size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={noParentValue}>{t.smartFolders.dialog.noParent}</SelectItem>
          {/* Keep the current parent selectable before candidate loading completes. */}
          {parent !== undefined &&
            parents.folders === undefined &&
            target.kind === "create" &&
            target.parent !== undefined && (
              <SelectItem value={String(target.parent.id)}>
                {target.parent.name}
              </SelectItem>
            )}
          {choices.map(({ folder, depth }) => (
            <SelectItem
              key={folder.id}
              value={String(folder.id)}
              title={folder.name}
              className={cn("*:[span]:last:min-w-0", indentClass(depth))}
            >
              <span className="truncate">{folder.name}</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error !== null && <FieldError>{error}</FieldError>}
      {error === null && parents.failure !== null && (
        <FieldError>{parents.failure}</FieldError>
      )}
    </Field>
  );
}

/**
 * Mount only while open and patch changed fields only. Missing parents reload choices; a missing
 * edited folder calls onGone, while other errors remain in the dialog.
 */
export default function SmartFolderDialog({
  target,
  onClose,
  onDone,
  onGone,
}: {
  target: SmartFolderDialogTarget;
  onClose: () => void;
  onDone: (folder: SmartFolderSummary) => void;
  onGone?: () => void;
}) {
  const nameId = useId();
  const nameErrorId = useId();
  const sortId = useId();
  const sortHintId = useId();
  const nameInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState(target.kind === "edit" ? target.folder.name : "");
  const [parent, setParent] = useState<number | undefined>(initialParent(target));
  const [sort, setSort] = useState<VideoSort>(target.sort);
  const [rule, setRule] = useState<SmartRule>(target.rule);
  const [errors, setErrors] = useState<FieldErrors>(noErrors);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);
  const parents = useParentFolders();
  const editing = target.kind === "edit" ? target.folder.id : undefined;
  const missing = target.kind === "edit" ? target.missing : [];
  const randomHint =
    target.kind === "create" && target.randomReplaced === true && sort === target.sort;

  useEffect(() => {
    if (target.kind === "edit") nameInput.current?.select();
  }, [target.kind]);

  const trimmedName = playlistName(name);
  // Validate and compare the normalized submitted rule; whitespace-only queries are neither conditions nor changes.
  const cleaned = sentRule(rule, target.rule);
  const draft = { name: trimmedName ?? name, parentId: parent, sort, rule: cleaned };
  const unchanged =
    target.kind === "edit" && Object.keys(editChange(target, draft)).length === 0;

  function check(): FieldErrors {
    const found: FieldErrors = { ...noErrors, name: trimmedName === null };
    const keepsEmpty = target.kind === "edit" && !hasRuleMember(target.rule);
    if (!hasRuleMember(cleaned) && !keepsEmpty) {
      found.conditions = t.smartFolders.dialog.noConditions;
    } else if (!validRule(cleaned)) {
      found.conditions = t.errors.code.invalid_smart_rule;
    }
    return found;
  }

  function send(): Promise<SmartFolderSummary> {
    if (target.kind === "create") {
      return createSmartFolder({
        name: trimmedName ?? "",
        rule: cleaned,
        sort,
        ...(parent === undefined ? {} : { parentId: parent }),
      });
    }
    return changeSmartFolder(
      target.folder.id,
      editChange(target, { ...draft, name: trimmedName ?? "" }),
    );
  }

  function failed(error: unknown) {
    if (
      target.kind === "edit" &&
      error instanceof RequestFailed &&
      error.status === 404 &&
      error.code === "not_found" &&
      onGone !== undefined
    ) {
      onGone();
      return;
    }
    const fieldErrors = failureErrors(error);
    if (fieldErrors === null) {
      setFailure(
        (target.kind === "create"
          ? t.smartFolders.dialog.createFailed
          : t.smartFolders.dialog.saveFailed)(errorText(error)),
      );
      return;
    }
    setErrors(fieldErrors);
    if (fieldErrors.name) nameInput.current?.focus();
    if (error instanceof RequestFailed && error.code === "smart_folder_not_found") {
      parents.reload();
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || unchanged) return;
    const found = check();
    setErrors(found);
    setFailure(null);
    if (found.name || found.conditions !== null) {
      if (found.name) nameInput.current?.focus();
      return;
    }
    setPending(true);
    send().then(
      (folder) => {
        setPending(false);
        onDone(folder);
      },
      (error: unknown) => {
        setPending(false);
        failed(error);
      },
    );
  }

  const words = submitWords(target);

  return (
    <FormDialog
      open
      size="wide"
      onOpenChange={(open) => {
        if (!open && !pending) onClose();
      }}
      title={dialogTitle(target)}
      onSubmit={submit}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? words.busy : words.idle}
        </>
      }
      pending={pending}
      submitDisabled={unchanged}
      initialFocus={nameInput}
    >
      <div className="grid gap-4 md:grid-cols-2">
        <Field data-invalid={errors.name || undefined}>
          <FieldLabel htmlFor={nameId}>{t.groups.nameLabel}</FieldLabel>
          <Input
            ref={nameInput}
            id={nameId}
            type="text"
            autoComplete="off"
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              setErrors((current) => ({ ...current, name: false }));
            }}
            aria-invalid={errors.name || undefined}
            aria-describedby={errors.name ? nameErrorId : undefined}
            className="h-8"
          />
          {errors.name && <FieldError id={nameErrorId}>{t.groups.nameRule}</FieldError>}
        </Field>
        <ParentField
          target={target}
          parent={parent}
          parents={parents}
          editing={editing}
          error={errors.parent}
          onChange={(next) => {
            setParent(next);
            setErrors((current) => ({ ...current, parent: null }));
          }}
        />
      </div>
      <Field>
        <FieldLabel htmlFor={sortId}>{t.list.sort.heading}</FieldLabel>
        <Select value={sort} onValueChange={(next) => setSort(next as VideoSort)}>
          <SelectTrigger
            id={sortId}
            size="sm"
            className="w-full md:w-1/2"
            aria-describedby={randomHint ? sortHintId : undefined}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {sortChoices().map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {randomHint && (
          <FieldDescription id={sortHintId}>
            {t.smartFolders.dialog.randomSort}
          </FieldDescription>
        )}
      </Field>
      <ConditionsFields
        rule={rule}
        onChange={(next) => {
          setRule(next);
          setErrors((current) => ({ ...current, conditions: null }));
        }}
        missing={missing}
        error={errors.conditions}
      />
      {failure !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle>{failure}</AlertTitle>
        </Alert>
      )}
    </FormDialog>
  );
}
