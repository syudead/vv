import { CircleDashed, Minus, Plus, X } from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  attachVideoTagByID,
  attachVideoTagByName,
  currentTags,
  detachVideoTag,
  refreshTags,
  subscribeTags,
  summarizeVideoTags,
  type Tag,
  type VideoTagsSummary,
} from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import Combobox, { type ComboboxOption } from "../ui/Combobox";
import IconButton from "../ui/IconButton";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import { useToast } from "../ui/Toast";

type VideoTagsSummaryItem = VideoTagsSummary["items"][number];

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

/** buildAddOptions は「タグを付ける」の候補（全タグ、各行の右に本数）を作る。 */
function buildAddOptions(
  allTags: readonly Tag[],
  input: string,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  let exactTag: Tag | undefined;
  for (const tag of allTags) {
    if (tag.name === trimmed || tag.synonyms.includes(trimmed)) {
      exactTag = tag;
      break;
    }
  }

  const matched = allTags
    .map((tag) => {
      const nameMatch = query === "" || tag.name.toLowerCase().includes(query);
      const synonymHit = tag.synonyms.find((synonym) =>
        synonym.toLowerCase().includes(query),
      );
      if (!nameMatch && synonymHit === undefined) return null;
      const namePrefix = query === "" || tag.name.toLowerCase().startsWith(query);
      const synonymPrefix =
        synonymHit !== undefined && synonymHit.toLowerCase().startsWith(query);
      return {
        tag,
        prefix: namePrefix || synonymPrefix,
        hint:
          !nameMatch && synonymHit !== undefined ? `シノニム: ${synonymHit}` : undefined,
      };
    })
    .filter((value): value is NonNullable<typeof value> => value !== null)
    .sort((a, b) => {
      if (a.prefix !== b.prefix) return a.prefix ? -1 : 1;
      return compareNatural(a.tag.name, b.tag.name);
    });

  const options: ComboboxOption[] = matched.map(({ tag, hint }) => ({
    id: String(tag.id),
    label: tag.name,
    hint,
    meta: `${String(tag.videoCount)} 本`,
  }));

  const exactOption: ComboboxOption | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: `${String(exactTag.videoCount)} 本`,
        };

  return { options, exactOption };
}

/** buildRemoveOptions は「タグを外す」の候補（要約のタグだけ）を作る。作成の行は持たない。 */
function buildRemoveOptions(
  summary: VideoTagsSummary,
  input: string,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  function toOption(item: VideoTagsSummaryItem): ComboboxOption {
    const partial = item.count < summary.total;
    return {
      id: String(item.tag.id),
      label: item.tag.name,
      meta: partial ? (
        <span className="inline-flex items-center gap-1">
          <CircleDashed className="size-3" aria-hidden="true" />
          <span>
            一部 {item.count} / {summary.total} 件
          </span>
        </span>
      ) : (
        `${String(summary.total)} 件`
      ),
      ariaLabel: partial
        ? `${item.tag.name}、一部の動画だけ、${String(summary.total)} 件中 ${String(item.count)} 件`
        : undefined,
    };
  }

  let exactItem: VideoTagsSummaryItem | undefined;
  for (const item of summary.items) {
    if (item.tag.name === trimmed) {
      exactItem = item;
      break;
    }
  }

  const matched = summary.items
    .filter((item) => query === "" || item.tag.name.toLowerCase().includes(query))
    .map((item) => ({
      item,
      prefix: query === "" || item.tag.name.toLowerCase().startsWith(query),
    }))
    .sort((a, b) => {
      if (a.prefix !== b.prefix) return a.prefix ? -1 : 1;
      return compareNatural(a.item.tag.name, b.item.tag.name);
    });

  return {
    options: matched.map(({ item }) => toOption(item)),
    exactOption: exactItem === undefined ? null : toOption(exactItem),
  };
}

/**
 * AddTagPopover は選択バーの「タグを付ける」の中身である
 * （ui-design.md「Selection bar」の「Add」）。
 */
function AddTagPopover({
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
  const [failed, setFailed] = useState(false);
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());

  useEffect(() => {
    const unsubscribe = subscribeTags((tags) => setAllTags(tags));
    return unsubscribe;
  }, []);

  // 開くたびに共有の一覧を取り直す（Plan の Structural Decisions 8。すでに保持が
  // あっても、別のタブでの変更に気付けるようにする）。
  useEffect(() => {
    if (!open) return;
    setValue("");
    setFailed(false);
    refreshTags().catch(() => undefined);
  }, [open]);

  const { options, exactOption } = buildAddOptions(allTags ?? [], value);
  const trimmed = value.trim();
  const createLabel =
    exactOption === null && trimmed !== "" ? (
      <span className="flex min-w-0 items-center gap-2">
        <Plus className="size-3.5 shrink-0 text-fg-muted" aria-hidden="true" />
        <span className="truncate">「{trimmed}」を作成</span>
      </span>
    ) : null;

  function submit(tag: { id: number; name: string } | { name: string }) {
    setFailed(false);
    setSubmitting(true);
    const displayName = tag.name;
    const request =
      "id" in tag
        ? attachVideoTagByID(selectedIds, tag.id)
        : attachVideoTagByName(selectedIds, tag.name);
    void request
      .then((result) => {
        toast(`${String(result.applied)} 件に「${displayName}」を付けました`);
        onOpenChange(false);
        onDone();
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(`タグ「${displayName}」はもう無いため、一覧を取り直しました`);
          refreshTags().catch(() => undefined);
          return;
        }
        setFailed(true);
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
        event.preventDefault();
        inputRef.current?.focus();
      }}
      onEscapeKeyDown={(event) => {
        if (listOpenRef.current) event.preventDefault();
      }}
    >
      <h2 id={headingId} className="sr-only">
        タグを付ける
      </h2>
      <Combobox
        value={value}
        onValueChange={setValue}
        options={options}
        exactOption={exactOption}
        onSelect={(option) => submit({ id: Number(option.id), name: option.label })}
        createLabel={createLabel}
        onCreate={(spelling) => submit({ name: spelling })}
        placeholder="タグを付ける"
        icon={<Plus className="size-3 shrink-0 text-fg-muted" aria-hidden="true" />}
        busy={submitting}
        side="top"
        aria-label="タグを付ける"
        inputRef={inputRef}
        onEscapeWhenClosed={() => onOpenChange(false)}
        onOpenChange={(listOpen) => {
          listOpenRef.current = listOpen;
        }}
        className="w-full"
        inputClassName="w-full"
        frameClassName="w-full"
      />
      {failed && (
        <p role="alert" className="mt-1 text-xs text-danger">
          付けられませんでした。もう一度お試しください
        </p>
      )}
    </PopoverContent>
  );
}

/**
 * RemoveTagPopover は選択バーの「タグを外す」の中身である
 * （ui-design.md「Selection bar」の「Remove」）。
 */
function RemoveTagPopover({
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
  // AddTagPopover と同じく、候補の一覧が開いているかを見張る（B2）。
  const listOpenRef = useRef(false);
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [failed, setFailed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [summary, setSummary] = useState<VideoTagsSummary | null>(null);

  const fetchSummary = useCallback(() => {
    setLoading(true);
    setFetchFailed(false);
    summarizeVideoTags(selectedIds)
      .then((result) => setSummary(result))
      .catch(() => setFetchFailed(true))
      .finally(() => setLoading(false));
  }, [selectedIds]);

  useEffect(() => {
    if (!open) return;
    setValue("");
    setFailed(false);
    setSummary(null);
    fetchSummary();
    // fetchSummary は selectedIds が変わるたびに作り直されるが、ここでは
    // ポップオーバーを開いた瞬間だけ呼べばよい（open の変化だけを見る）。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // 「読み込み中…」から Combobox に切り替わった瞬間（要約が届いたとき）は、
  // ui-design.md「Add」「Remove」と同じくフォーカスを入力へ移す。
  // `onOpenAutoFocus` は最初のマウント時にしか働かないため、ここで補う。
  const ready = open && !loading && !fetchFailed && (summary?.items.length ?? 0) > 0;
  useEffect(() => {
    if (ready) inputRef.current?.focus();
  }, [ready]);

  const { options, exactOption } =
    summary === null
      ? { options: [], exactOption: null }
      : buildRemoveOptions(summary, value);

  function submit(option: ComboboxOption) {
    setFailed(false);
    setSubmitting(true);
    const tagId = Number(option.id);
    const displayName = option.label;
    void detachVideoTag(selectedIds, tagId)
      .then((result) => {
        toast(`${String(result.applied)} 件から「${displayName}」を外しました`);
        setValue("");
        fetchSummary();
        onDone();
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(`タグ「${displayName}」はもう無いため、一覧を取り直しました`);
          fetchSummary();
          return;
        }
        setFailed(true);
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
        if (loading || fetchFailed || summary?.items.length === 0) event.preventDefault();
        else inputRef.current?.focus();
      }}
      onEscapeKeyDown={(event) => {
        if (listOpenRef.current) event.preventDefault();
      }}
    >
      <h2 id={headingId} className="sr-only">
        タグを外す
      </h2>
      {loading && (
        <p role="status" className="text-xs text-fg-muted">
          読み込み中…
        </p>
      )}
      {!loading && fetchFailed && (
        <div className="flex flex-col items-start gap-2">
          <p role="alert" className="text-xs text-danger">
            タグを取得できませんでした
          </p>
          <Button variant="ghost" size="sm" onClick={fetchSummary}>
            再試行
          </Button>
        </div>
      )}
      {!loading && !fetchFailed && summary !== null && summary.items.length === 0 && (
        <p className="text-xs text-fg-muted">選んだ動画にタグはありません</p>
      )}
      {!loading && !fetchFailed && summary !== null && summary.items.length > 0 && (
        <>
          <Combobox
            value={value}
            onValueChange={setValue}
            options={options}
            exactOption={exactOption}
            onSelect={submit}
            createLabel={null}
            placeholder="タグを外す"
            icon={<Minus className="size-3 shrink-0 text-fg-muted" aria-hidden="true" />}
            busy={submitting}
            side="top"
            aria-label="タグを外す"
            inputRef={inputRef}
            onEscapeWhenClosed={() => onOpenChange(false)}
            onOpenChange={(listOpen) => {
              listOpenRef.current = listOpen;
            }}
            className="w-full"
            inputClassName="w-full"
            frameClassName="w-full"
          />
          {failed && (
            <p role="alert" className="mt-1 text-xs text-danger">
              外せませんでした。もう一度お試しください
            </p>
          )}
        </>
      )}
    </PopoverContent>
  );
}

export interface SelectionBarProps {
  count: number;
  /** 今の条件に合う全件数（サーバーの total）。「すべて選択」の disabled 判定に使う。 */
  total: number;
  selectedIds: readonly number[];
  /** 「すべて選択」の要求の間 true（ui-design.md「Selection bar」の「Layout」）。 */
  selectingAll: boolean;
  onSelectAll: () => void;
  onClear: () => void;
}

/** SelectionBar は 1 件以上選ぶと画面下部に浮く。 */
export default function SelectionBar({
  count,
  total,
  selectedIds,
  selectingAll,
  onSelectAll,
  onClear,
}: SelectionBarProps) {
  const [addOpen, setAddOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);
  const addTriggerRef = useRef<HTMLButtonElement | null>(null);

  if (count === 0) return null;

  return (
    <div
      role="region"
      aria-label="選択中の操作"
      className="fixed inset-x-0 bottom-4 z-30 flex justify-center px-4"
    >
      <div className="flex w-full flex-wrap items-center gap-x-2 gap-y-1.5 rounded-md bg-elevated p-1.5 shadow-elevated animate-slide-up sm:h-11 sm:w-auto sm:flex-nowrap sm:py-0 sm:pr-1.5 sm:pl-4">
        <span
          role="status"
          aria-live="polite"
          className="order-1 px-1 text-sm text-fg tabular-nums sm:px-0"
        >
          {count.toLocaleString("ja-JP")} 件を選択中
        </span>

        {/*
         * sm 未満では常に2段にする（B3）。内容がたまたま1行に収まる幅
         * （390〜600px など）でも、この行幅いっぱいの見えない仕切りが flex-wrap
         * を強制する。sm 以上では display:none になり、何も強制しない。
         */}
        <span
          aria-hidden="true"
          className="order-4 hidden max-sm:block max-sm:h-0 max-sm:w-full max-sm:basis-full"
        />

        <PopoverRoot open={addOpen} onOpenChange={setAddOpen}>
          <PopoverTrigger asChild>
            <Button
              ref={addTriggerRef}
              variant="ghost"
              size="sm"
              className="order-5 max-sm:flex-1 sm:order-2"
            >
              <Plus aria-hidden="true" />
              タグを付ける
            </Button>
          </PopoverTrigger>
          <AddTagPopover
            open={addOpen}
            onOpenChange={setAddOpen}
            selectedIds={selectedIds}
            onDone={() => addTriggerRef.current?.focus()}
          />
        </PopoverRoot>

        <PopoverRoot open={removeOpen} onOpenChange={setRemoveOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="order-6 max-sm:flex-1 sm:order-3"
            >
              <Minus aria-hidden="true" />
              タグを外す
            </Button>
          </PopoverTrigger>
          <RemoveTagPopover
            open={removeOpen}
            onOpenChange={setRemoveOpen}
            selectedIds={selectedIds}
            onDone={() => undefined}
          />
        </PopoverRoot>

        <span
          aria-hidden="true"
          className={cn("hidden h-5 w-px bg-border-strong sm:order-4 sm:block")}
        />

        <Button
          variant="ghost"
          size="sm"
          onClick={onSelectAll}
          disabled={selectingAll || count >= total}
          className="order-2 sm:order-5"
        >
          {selectingAll ? "選択中…" : "すべて選択"}
        </Button>
        <IconButton
          label="選択を解除 (Esc)"
          size="sm"
          onClick={onClear}
          className="order-3 sm:order-6"
        >
          <X />
        </IconButton>
      </div>
    </div>
  );
}
