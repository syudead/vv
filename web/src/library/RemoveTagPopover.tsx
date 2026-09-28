import { Minus } from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";

import {
  detachVideoTag,
  maxVideoTagsSelection,
  summarizeVideoTags,
  type VideoTagsSummary,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import Combobox, { type ComboboxOption } from "../ui/Combobox";
import { PopoverContent } from "../ui/Popover";
import { useToast } from "../ui/Toast";
import { isTagNotFound, overLimitMessage } from "./selectionErrors";
import { buildRemoveOptions, removableSummary } from "./tagChoices";

/**
 * RemoveTagPopover は選択バーの「タグを外す」の中身である
 * （ui-design.md「Selection bar」の「Remove」）。
 */
export default function RemoveTagPopover({
  open,
  onOpenChange,
  selectedIds,
  onRemoved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedIds: readonly number[];
  /** タグを外し終えるたびに、外したタグの id を渡して呼ぶ。 */
  onRemoved: (tagId: number) => void;
}) {
  const toast = useToast();
  const headingId = useId();
  const inputRef = useRef<HTMLInputElement | null>(null);
  // AddTagPopover と同じく、候補の一覧が開いているかを見張る（B2）。
  const listOpenRef = useRef(false);
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<UiText | null>(null);
  const [loading, setLoading] = useState(false);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [summary, setSummary] = useState<VideoTagsSummary | null>(null);

  // 開いている間に選択が増えて上限を超えたら、要約の取得も送信も止める
  // （親の SelectionBar がこのポップオーバー自体を閉じるまでの間の保険）。
  const overLimit = selectedIds.length > maxVideoTagsSelection;

  // 要約の取得は選ぶたびに通し番号を払い出し、古い応答が新しい選択の結果を
  // 上書きしないようにする（Devin の指摘2。summarizeVideoTags 自身は要求の
  // 順・届く順を揃えない）。
  const summarySeq = useRef(0);
  const fetchSummary = useCallback(() => {
    // 選択が空になった直後（一括で外したタグが今の絞り込みに含まれていて、
    // 呼び出し元が選択を解除した直後など）は、このポップオーバーはすぐ閉じる
    // ので要求しない。`POST /api/video-tags/summary` は videoIds を1件以上
    // 要る（contracts/tags-api.md §4）。
    if (selectedIds.length === 0) return;
    if (selectedIds.length > maxVideoTagsSelection) {
      // 進行中の（上限内だった頃に始めた）取得を無効にする。その応答が
      // 後から届いても、上限超過の表示を古い要約で上書きしない。
      summarySeq.current += 1;
      setLoading(false);
      setFetchFailed(false);
      setSummary(null);
      return;
    }
    const seq = (summarySeq.current += 1);
    setLoading(true);
    setFetchFailed(false);
    summarizeVideoTags(selectedIds)
      .then((result) => {
        if (summarySeq.current !== seq) return;
        setSummary(removableSummary(result));
      })
      .catch(() => {
        if (summarySeq.current !== seq) return;
        setFetchFailed(true);
      })
      .finally(() => {
        if (summarySeq.current === seq) setLoading(false);
      });
  }, [selectedIds]);

  // 開いている間に選択が変わったら（別の動画を選び直す・「すべて選択」の
  // 結果が届くなど）要約を取り直す。取り直している間は loading が立ち、
  // 候補（Combobox）ごと隠れるので、古い候補を選べない（Devin の指摘2）。
  // 開いた瞬間（justOpened）だけ、前回の入力と失敗の文言を消す。
  const wasOpenRef = useRef(false);
  useEffect(() => {
    if (!open) {
      wasOpenRef.current = false;
      return;
    }
    const justOpened = !wasOpenRef.current;
    wasOpenRef.current = true;
    if (justOpened) {
      setValue("");
      setErrorMessage(null);
    }
    setSummary(null);
    fetchSummary();
  }, [open, fetchSummary]);

  // 「読み込み中…」から Combobox に切り替わった瞬間（要約が届いたとき）は、
  // ui-design.md「Add」「Remove」と同じくフォーカスを入力へ移す。
  // `onOpenAutoFocus` は最初のマウント時にしか働かないため、ここで補う。
  const ready =
    open && !overLimit && !loading && !fetchFailed && (summary?.items.length ?? 0) > 0;
  useEffect(() => {
    if (ready) inputRef.current?.focus();
  }, [ready]);

  const { options, exactOption } =
    summary === null
      ? { options: [], exactOption: null }
      : buildRemoveOptions(summary, value);

  function submit(option: ComboboxOption) {
    setErrorMessage(null);
    // 送る直前に selectedIds の最新の件数を確かめる（AddTagPopover.submit と
    // 同じ理由。contracts/tags-api.md §4）。
    if (selectedIds.length > maxVideoTagsSelection) {
      setErrorMessage(overLimitMessage());
      return;
    }
    setSubmitting(true);
    const tagId = Number(option.id);
    const displayName = option.label;
    void detachVideoTag(selectedIds, tagId)
      .then((result) => {
        toast(t.library.selection.removed(result.applied, displayName));
        setValue("");
        fetchSummary();
        onRemoved(tagId);
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(t.library.selection.tagGone(displayName));
          fetchSummary();
          return;
        }
        setErrorMessage(t.library.selection.removeFailed(errorText(error)));
      })
      .finally(() => setSubmitting(false));
  }

  return (
    <PopoverContent
      side="top"
      align="start"
      aria-labelledby={headingId}
      className="w-72 p-3"
      onOpenAutoFocus={(event) => {
        if (overLimit || loading || fetchFailed || summary?.items.length === 0) {
          event.preventDefault();
        } else {
          inputRef.current?.focus();
        }
      }}
      onEscapeKeyDown={(event) => {
        if (listOpenRef.current) event.preventDefault();
      }}
    >
      <h2 id={headingId} className="sr-only">
        {t.library.selection.removeTag}
      </h2>
      {overLimit && (
        <p role="alert" className="text-xs text-danger">
          {overLimitMessage()}
        </p>
      )}
      {!overLimit && loading && (
        <p role="status" className="text-xs text-fg-muted">
          {t.library.selection.loading}
        </p>
      )}
      {!overLimit && !loading && fetchFailed && (
        <div className="flex flex-col items-start gap-2">
          <p role="alert" className="text-xs text-danger">
            {t.library.selection.summaryFailed}
          </p>
          <Button variant="ghost" size="sm" onClick={fetchSummary}>
            {t.common.retry}
          </Button>
        </div>
      )}
      {!overLimit &&
        !loading &&
        !fetchFailed &&
        summary !== null &&
        summary.items.length === 0 && (
          <p className="text-xs text-fg-muted">{t.library.selection.nothingToRemove}</p>
        )}
      {!overLimit &&
        !loading &&
        !fetchFailed &&
        summary !== null &&
        summary.items.length > 0 && (
          <>
            <Combobox
              value={value}
              onValueChange={setValue}
              options={options}
              exactOption={exactOption}
              onSelect={submit}
              createLabel={null}
              placeholder={t.library.selection.removeTag}
              icon={
                <Minus className="size-3 shrink-0 text-fg-muted" aria-hidden="true" />
              }
              busy={submitting}
              side="top"
              aria-label={t.library.selection.removeTag}
              inputRef={inputRef}
              onEscapeWhenClosed={() => onOpenChange(false)}
              onOpenChange={(listOpen) => {
                listOpenRef.current = listOpen;
              }}
              className="w-full"
              inputClassName="w-full"
              frameClassName="w-full"
            />
            {errorMessage !== null && (
              <p role="alert" className="mt-1 text-xs text-danger">
                {errorMessage}
              </p>
            )}
          </>
        )}
    </PopoverContent>
  );
}
