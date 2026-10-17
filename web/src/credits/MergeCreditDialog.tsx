import { useEffect, useEffectEvent, useRef, useState } from "react";

import { isAborted } from "../api/client";
import {
  type Credit,
  type CreditKind,
  isCreditGone,
  listCredits,
  mergeCredits,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { DialogError } from "../tags/DialogError";
import { MergeSummary, MergeTargetField } from "../tags/MergeParts";
import CreditCommand, { type CreditChoice } from "../ui/CreditCommand";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { kindTexts } from "./creditRows";

/** candidateLimit は統合先の候補に出す最大の行数である。 */
const candidateLimit = 8;

/**
 * MergeCreditDialog は人物・団体の統合の窓である。タグの統合の窓（036）の統合元 1 件の形で、統合先は
 * `CreditCommand` の `inline` で選ぶ。候補は入力のたびに `GET /api/people?q=&limit=…` で
 * 引き、統合元を除き、作成の行は出さない。統合先を選ぶと確認の文（統合元の
 * `videoCount`）と「統合元 → 統合先」を出し、「Merge」を押せるようにする。
 *
 * 統合先か統合元がもう無いときは `onStale`（窓を閉じて一覧を取り直す）。親の輪
 * （`organization_parent_cycle`）と別名・外部 ID の上限（`work_info_too_many`）は窓の中の
 * 失敗の 1 行に出し、何も変えない。
 */
export default function MergeCreditDialog({
  kind,
  source,
  onClose,
  onMerged,
  onStale,
}: {
  kind: CreditKind;
  source: Credit;
  onClose: () => void;
  /** 統合が成功したときに、統合先の最新の状態と統合の前の統合先（選んだ候補）を渡す。 */
  onMerged: (merged: Credit, target: Credit) => void;
  onStale: () => void;
}) {
  const texts = kindTexts(kind);
  const cancel = useRef<HTMLButtonElement>(null);
  const mergeButton = useRef<HTMLButtonElement>(null);
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Credit | null>(null);
  const [candidates, setCandidates] = useState<readonly Credit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  // 候補を選んだ直後だけ「Merge」へフォーカスを移す。
  const justSelectedRef = useRef(false);

  // 入力が変わるたびに候補を引く（開いた直後の空の入力も同じ経路）。統合元の分を多く引いて
  // 8 件に保つ。進行中の要求は次の入力で打ち切る。
  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listCredits(
      kind,
      { q: value.trim(), limit: candidateLimit + 1 },
      controller.signal,
    ).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setSearching(false);
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setSearchError(errorText(failure));
        setSearching(false);
      },
    );
    return () => controller.abort();
  }, [kind, value]);

  const shown = candidates
    .filter((credit) => credit.id !== source.id)
    .slice(0, candidateLimit);
  const folded = foldForMatch(value.trim());
  const options: CreditChoice[] = shown.map((credit) => {
    const alias =
      folded === "" || foldForMatch(credit.name).includes(folded)
        ? undefined
        : credit.aliases.find((item) => foldForMatch(item.name).includes(folded))?.name;
    return {
      id: String(credit.id),
      label: credit.name,
      hint: alias === undefined ? undefined : t.credits.mergeDialog.aliasHint(alias),
      meta: t.credits.mergeDialog.videoCount(credit.videoCount),
    };
  });
  const trimmed = value.trim();
  const exact = shown.find((credit) => trimmed !== "" && credit.name === trimmed);
  const exactOption =
    exact === undefined
      ? null
      : (options.find((option) => option.id === String(exact.id)) ?? null);

  const canSubmit = target !== null && !pending;

  useEffect(() => {
    if (justSelectedRef.current && target !== null) {
      justSelectedRef.current = false;
      mergeButton.current?.focus();
    }
  }, [target]);

  // 統合が失敗した直後は、まだ押せる操作へフォーカスを戻す（タグの統合の窓と同じ）。
  const restoreFocus = useEffectEvent(() => {
    (target !== null ? mergeButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (error !== null) restoreFocus();
  }, [error]);

  function handleClose() {
    if (pending) return;
    onClose();
  }

  function selectTarget(option: CreditChoice) {
    const found = shown.find((credit) => String(credit.id) === option.id);
    if (found === undefined) return;
    justSelectedRef.current = true;
    setTarget(found);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || !canSubmit) return;
    setError(null);
    setPending(true);
    try {
      const { credit: merged, notFoundIds } = await mergeCredits(kind, target.id, [
        source.id,
      ]);
      // 統合元がもう無かったときは何も統合されていない。
      if (notFoundIds.includes(source.id)) {
        onStale();
        return;
      }
      onMerged(merged, target);
    } catch (failure) {
      if (isCreditGone(failure)) {
        onStale();
        return;
      }
      setError(errorText(failure));
      setPending(false);
    }
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      title={t.credits.mergeDialog.title(source.name)}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.credits.mergeDialog.submitting : t.credits.mergeDialog.submit}
        </>
      }
      pending={pending}
      submitDisabled={!canSubmit}
      submitRef={mergeButton}
      cancelRef={cancel}
      initialFocus={cancel}
    >
      <MergeTargetField
        Command={CreditCommand}
        label={texts.mergeTarget}
        value={value}
        onValueChange={(next) => {
          setValue(next);
          setTarget(null);
          setError(null);
        }}
        choices={options}
        exactChoice={exactOption}
        onSelect={selectTarget}
        chosenId={target === null ? undefined : String(target.id)}
        searching={searching}
        searchError={searchError === null ? null : texts.searchFailed(searchError)}
        emptyText={texts.noCandidates}
      />
      {target !== null && (
        <>
          <p className="text-sm text-muted-foreground">
            {t.credits.mergeDialog.warning(source.name, source.videoCount, target.name)}
          </p>
          <MergeSummary source={source.name} target={target.name} />
        </>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}
