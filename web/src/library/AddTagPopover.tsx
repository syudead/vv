import { Plus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import {
  attachVideoTagByID,
  attachVideoTagByName,
  currentTags,
  maxVideoTagsSelection,
  refreshTags,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import Combobox from "../ui/legacy/Combobox";
import { PopoverContent } from "../ui/Popover";
import { useToast } from "../ui/Toast";
import { isTagNotFound, overLimitMessage } from "./selectionErrors";
import { buildAddOptions } from "./tagChoices";

/**
 * AddTagPopover は選択バーの「タグを付ける」の中身である
 * （ui-design.md「Selection bar」の「Add」）。
 */
export default function AddTagPopover({
  open,
  onOpenChange,
  selectedIds,
  onDone,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedIds: readonly number[];
  onDone: () => void;
}) {
  const toast = useToast();
  const headingId = useId();
  const inputRef = useRef<HTMLInputElement | null>(null);
  // Radix の DismissableLayer は document の capture 段階で Esc を先に拾う
  // ため、combobox の候補の一覧が開いているかをここで見張り、開いていれば
  // PopoverContent の onEscapeKeyDown で既定の「閉じる」を止める（B2）。
  const listOpenRef = useRef(false);
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<UiText | null>(null);
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());

  useEffect(() => {
    const unsubscribe = subscribeTags((tags) => setAllTags(tags));
    return unsubscribe;
  }, []);

  // 開くたびに共有の一覧を確かめ直す（直前に届いた一覧があればそれを使う）。
  useEffect(() => {
    if (!open) return;
    setValue("");
    setErrorMessage(null);
    revalidateTags().catch(() => undefined);
  }, [open]);

  // 開いている間に選択が増えて上限を超えたら、候補も送信も止める（親の
  // SelectionBar がこのポップオーバー自体を閉じるまでの間の保険）。
  const overLimit = selectedIds.length > maxVideoTagsSelection;

  const { options, exactOption } = buildAddOptions(allTags ?? [], value);
  const trimmed = value.trim();
  const createLabel =
    exactOption === null && trimmed !== "" ? (
      <span className="flex min-w-0 items-center gap-2">
        <Plus className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{t.library.selection.create(trimmed)}</span>
      </span>
    ) : null;

  function submit(tag: { id: number; name: string } | { name: string }) {
    setErrorMessage(null);
    // 送る直前に selectedIds の最新の件数を確かめる。ポップオーバーを開いた
    // ままの間に選択が増えて上限を超えていれば、静かに送らず理由を示す
    // （contracts/tags-api.md §4: 全部か無しかで、超えると400になる）。
    if (selectedIds.length > maxVideoTagsSelection) {
      setErrorMessage(overLimitMessage());
      return;
    }
    setSubmitting(true);
    const displayName = tag.name;
    const request =
      "id" in tag
        ? attachVideoTagByID(selectedIds, tag.id)
        : attachVideoTagByName(selectedIds, tag.name);
    void request
      .then((result) => {
        toast(t.library.selection.added(result.applied, displayName));
        onOpenChange(false);
        onDone();
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(t.library.selection.tagGone(displayName));
          refreshTags().catch(() => undefined);
          return;
        }
        setErrorMessage(t.library.selection.addFailed(errorText(error)));
      })
      .finally(() => setSubmitting(false));
  }

  return (
    <PopoverContent
      side="top"
      align="start"
      aria-labelledby={headingId}
      className="w-popover p-3"
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        if (!overLimit) inputRef.current?.focus();
      }}
      onEscapeKeyDown={(event) => {
        if (listOpenRef.current) event.preventDefault();
      }}
    >
      <h2 id={headingId} className="sr-only">
        {t.library.selection.addTag}
      </h2>
      {overLimit ? (
        <p role="alert" className="text-xs text-destructive">
          {overLimitMessage()}
        </p>
      ) : (
        <>
          <Combobox
            value={value}
            onValueChange={setValue}
            options={options}
            exactOption={exactOption}
            onSelect={(option) => submit({ id: Number(option.id), name: option.label })}
            createLabel={createLabel}
            onCreate={(spelling) => submit({ name: spelling })}
            placeholder={t.library.selection.addTag}
            icon={
              <Plus
                className="size-3 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            }
            busy={submitting}
            side="top"
            aria-label={t.library.selection.addTag}
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
            <p role="alert" className="mt-1 text-xs text-destructive">
              {errorMessage}
            </p>
          )}
        </>
      )}
    </PopoverContent>
  );
}
