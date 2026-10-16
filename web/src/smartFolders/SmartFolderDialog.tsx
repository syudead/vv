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

/** SmartFolderDialogTarget は窓が作るか直すものである。 */
export type SmartFolderDialogTarget =
  | {
      kind: "create";
      /** 条件の初めの値（ライブラリの今の絞り込み、新しい子なら空）。 */
      rule: SmartRule;
      sort: VideoSort;
      /** ランダムの並べ順を追加日の新しい順に置き換えた（並べ順の欄に添え書きを出す）。 */
      randomReplaced?: boolean;
      /** 新しい子の親（フォルダのページの「New child smart folder…」）。 */
      parent?: { id: number; name: string };
    }
  | {
      kind: "edit";
      folder: { id: number; name: string; parentId?: number };
      rule: SmartRule;
      sort: VideoSort;
      /** もう無い参照（SmartFolderSettings.missing）。 */
      missing: readonly SmartRuleRef[];
    };

/**
 * indentClasses は親の `Select` の行の字下げで、深さ 1 段ごとに `pl-4` ずつ増やす
 * （ui-design.md「Smart folder dialog」の Parent）。4 段より深いものは 4 段目にそろえる。
 */
const indentClasses = ["pl-2", "pl-6", "pl-10", "pl-14", "pl-18"] as const;

function indentClass(depth: number): string {
  return indentClasses[Math.min(depth, indentClasses.length - 1)] ?? "pl-2";
}

/** noParentValue は親の `Select` の「None (top level)」の値である（id は正の整数）。 */
const noParentValue = "none";

/** sortChoices は並べ順の選択肢（種類と向き、ランダムを除く）である。 */
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

/** FieldErrors は欄ごとの誤りである。 */
interface FieldErrors {
  name: boolean;
  parent: UiText | null;
  conditions: UiText | null;
}

const noErrors: FieldErrors = { name: false, parent: null, conditions: null };

/**
 * failureErrors は送った結果の失敗を、どの欄の誤りかに振り分ける
 * （contracts/screen-api.md「Statuses」、ui-design.md「Words」）。欄の無い失敗は null。
 */
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

/**
 * useParentFolders は親の `Select` に並べるフォルダを `GET /api/smart-folders` で読む。
 * `reload` で読み直す（無い親を選んで `404` だったとき）。
 */
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

/** dialogTitle は窓の題である。 */
function dialogTitle(target: SmartFolderDialogTarget): UiText {
  if (target.kind === "edit") return t.smartFolders.dialog.editTitle;
  return target.parent === undefined
    ? t.smartFolders.dialog.newTitle
    : t.smartFolders.dialog.newChildTitle(target.parent.name);
}

function initialParent(target: SmartFolderDialogTarget): number | undefined {
  return target.kind === "edit" ? target.folder.parentId : target.parent?.id;
}

/**
 * editChange は直す窓の、元から変わった欄だけの変更である。`draft.rule` は送る形に
 * 整えた条件（sentRule）なので、整えて元と同じになる編集は変更にならない。
 */
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

/**
 * sentRule は送る条件である。検索語は手で変えたときだけライブラリと同じく整える（契約が
 * 許す 500 文字の値を、触らずに切らない）。
 */
function sentRule(rule: SmartRule, original: SmartRule): SmartRule {
  const q = rule.q === original.q ? rule.q : normalizeQuery(rule.q ?? "");
  return cleanRule({ ...rule, q });
}

/** submitWords は送るボタンの、待っていないときと送っている間の言葉である。 */
function submitWords(target: SmartFolderDialogTarget): { idle: UiText; busy: UiText } {
  return target.kind === "create"
    ? { idle: t.smartFolders.dialog.submit, busy: t.smartFolders.dialog.submitting }
    : { idle: t.smartFolders.dialog.saveSubmit, busy: t.smartFolders.dialog.saving };
}

/**
 * ParentField は親を選ぶ欄である（ui-design.md「Smart folder dialog」の Parent）。直す窓では
 * フォルダ自身とその子孫を選べない（parentChoices）。欄の誤り error が無いときは、親の一覧を
 * 読めなかったことを出す。
 */
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
  /** 直す窓のフォルダの id（作る窓は undefined）。 */
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
          {/* 一覧が届く前も、今の親を選んだ形で出せるように 1 行置く。 */}
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
 * SmartFolderDialog はスマートフォルダを作るか直す窓である
 * （specs/052-smart-folders/ui-design.md「Smart folder dialog」）。名前・親・並べ順と条件を持つ。
 * 呼び出し元は開いている間だけ描く。
 *
 * - 名前（1〜100 文字）と条件の 1 つ（直すときは、元から空の条件はそのまま）を送る前に
 *   確かめ、足りなければ送らずに欄の誤りを出す。範囲の外の条件も送らない。
 * - 直すときは何かが変わるまで「Save」を押せない。変わった欄だけを送る。
 * - `400`・`404`・`409` はその欄の誤りに、ほかの失敗は窓の中の `Alert` に出し、窓は開いたまま。
 *   無い親（`404 smart_folder_not_found`）のときは親の一覧を読み直す。
 * - 直す窓でフォルダ自身が無い（`404 not_found`）ときは `onGone` を呼ぶ。
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

  // 直す窓は名前を選んだ状態で開く（ui-design.md「Editing」）。
  useEffect(() => {
    if (target.kind === "edit") nameInput.current?.select();
  }, [target.kind]);

  const trimmedName = playlistName(name);
  // 確かめるのも、変わったかを比べるのも、送る形に整えた条件で行う（空白だけの検索語は
  // 条件にならず、整えて元と同じ検索語は変更にならない）。
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
