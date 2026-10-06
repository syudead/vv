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
import { PopoverContent } from "../ui/shadcn/popover";
import { useToast } from "../ui/Toast";
import { isTagNotFound, overLimitMessage } from "./selectionErrors";
import { buildAddOptions } from "./tagChoices";
import TagCommand from "../ui/TagCommand";

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
        <Plus className="text-muted-foreground" aria-hidden="true" />
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
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        if (!overLimit) inputRef.current?.focus();
      }}
      // Esc はポップオーバーだけを閉じ、選択は残す。既定を止めて自分で閉じ、一覧の
      // Esc（選択の解除）に「使った」と伝える。
      onEscapeKeyDown={(event) => {
        event.preventDefault();
        onOpenChange(false);
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
          <TagCommand
            label={t.library.selection.addTag}
            value={value}
            onValueChange={setValue}
            choices={options}
            exactChoice={exactOption}
            onSelect={(option) => submit({ id: Number(option.id), name: option.label })}
            createLabel={createLabel}
            onCreate={(spelling) => submit({ name: spelling })}
            icon={<Plus aria-hidden="true" />}
            busy={submitting}
            inputRef={inputRef}
          />
          {errorMessage !== null && (
            <p role="alert" className="text-xs text-destructive">
              {errorMessage}
            </p>
          )}
        </>
      )}
    </PopoverContent>
  );
}
