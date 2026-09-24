import { Plus, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { RequestFailed, type TagRef } from "../api/client";
import {
  attachVideoTagByID,
  attachVideoTagByName,
  currentTags,
  detachVideoTag,
  refreshTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { applyTagToTags, compareNatural } from "../api/tagOrder";
import { subscribeVideoTags } from "../api/videoTagsEvents";
import { cn } from "../lib/cn";
import Combobox, { type ComboboxOption } from "../ui/Combobox";
import { useToast } from "../ui/Toast";

/**
 * VideoTags は再生画面の題名の下のタグの並びである（要件 1・2、
 * ui-design.md「Video page tags」）。付け外しはサーバーの応答を受けてから
 * 反映するので、共有のタグの付け外しの通知（videoTagsEvents）を購読して
 * 一覧を直す。
 */
export default function VideoTags({
  videoId,
  tags: initialTags,
  onStaleVideo,
}: {
  videoId: number;
  tags: readonly TagRef[];
  /** タグがもう無い（tag_not_found）ときに、この動画を取り直すために呼ぶ。 */
  onStaleVideo: () => void;
}) {
  const toast = useToast();

  const [tags, setTags] = useState<readonly TagRef[]>(initialTags);
  useEffect(() => setTags(initialTags), [initialTags]);
  useEffect(
    () =>
      subscribeVideoTags((videoIds, tag, action) => {
        if (!videoIds.includes(videoId)) return;
        setTags((current) => applyTagToTags(current, tag, action));
      }),
    [videoId],
  );

  // 画面が開くときは、共有の保持がすでにあっても必ず取り直す（plan の
  // Structural Decisions 8「画面が開くとき…に refreshTags で取り直す」）。
  // 取り直す間は、あれば直近の保持を初期値として先に出す。
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  useEffect(() => {
    let alive = true;
    refreshTags()
      .then((loaded) => {
        if (alive) setAllTags(loaded);
      })
      .catch(() => undefined);
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setAllTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, []);

  const [inputValue, setInputValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [removingIds, setRemovingIds] = useState<ReadonlySet<number>>(new Set());
  const [opError, setOpError] = useState<"attach" | "detach" | null>(null);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const buttonRefs = useRef(new Map<number, HTMLButtonElement>());
  const pendingFocusRef = useRef<{ chipId: number } | "input" | null>(null);

  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending === null) return;
    // 外せなかったときの戻し先は、まだ disabled のままの、そのチップ自身の ×
    // かもしれない。再び押せるようになるまで（removingIds から消えるまで）待つ。
    if (pending !== "input" && removingIds.has(pending.chipId)) return;
    pendingFocusRef.current = null;
    if (pending === "input") {
      inputRef.current?.focus();
      return;
    }
    buttonRefs.current.get(pending.chipId)?.focus();
  }, [tags, removingIds]);

  const attachedIds = new Set(tags.map((tag) => tag.id));
  const { options, exactOption } = buildOptions(allTags ?? [], attachedIds, inputValue);

  function isTagNotFound(error: unknown): boolean {
    return error instanceof RequestFailed && error.code === "tag_not_found";
  }

  function submitAdd(tag: { id: number; name: string } | { name: string }) {
    setOpError(null);
    setSubmitting(true);
    const displayName = tag.name;
    const request =
      "id" in tag
        ? attachVideoTagByID([videoId], tag.id)
        : attachVideoTagByName([videoId], tag.name);
    void request
      .then(() => setInputValue(""))
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(`タグ「${displayName}」はもう無いため、一覧を取り直しました`);
          onStaleVideo();
          return;
        }
        setOpError("attach");
      })
      .finally(() => setSubmitting(false));
  }

  function removeTag(tag: TagRef, index: number) {
    const next = tags[index + 1] ?? tags[index - 1];
    pendingFocusRef.current = next !== undefined ? { chipId: next.id } : "input";
    setOpError(null);
    setRemovingIds((current) => new Set(current).add(tag.id));
    void detachVideoTag([videoId], tag.id)
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(`タグ「${tag.name}」はもう無いため、一覧を取り直しました`);
          onStaleVideo();
          return;
        }
        // 外せなかったときは、次/前のチップではなく、このチップの × へ戻す。
        pendingFocusRef.current = { chipId: tag.id };
        setOpError("detach");
      })
      .finally(() => {
        setRemovingIds((current) => {
          const next = new Set(current);
          next.delete(tag.id);
          return next;
        });
      });
  }

  const trimmed = inputValue.trim();
  const createLabel =
    exactOption === null && trimmed !== "" ? (
      <span className="flex min-w-0 items-center gap-2">
        <Plus className="size-3.5 shrink-0 text-fg-muted" aria-hidden="true" />
        <span className="truncate">「{trimmed}」を作成</span>
      </span>
    ) : null;

  return (
    <div className="flex flex-col gap-1">
      <h2 className="sr-only">タグ</h2>
      <ul className="flex flex-wrap items-center gap-1.5">
        {tags.map((tag, index) => (
          <li key={tag.id} className="min-w-0 max-w-full">
            <span
              title={tag.name}
              className="inline-flex h-6 max-w-full items-center rounded-sm bg-elevated pl-2 text-xs text-fg"
            >
              <Link
                to={`/?tag=${String(tag.id)}`}
                aria-label={`${tag.name}で絞り込む`}
                className="min-w-0 truncate hover:text-link"
              >
                {tag.name}
              </Link>
              <span aria-hidden="true" className="mx-1.5 h-3.5 w-px bg-border-strong" />
              <button
                ref={(node) => {
                  if (node) buttonRefs.current.set(tag.id, node);
                  else buttonRefs.current.delete(tag.id);
                }}
                type="button"
                aria-label={`${tag.name}をこの動画から外す`}
                disabled={removingIds.has(tag.id)}
                onClick={() => removeTag(tag, index)}
                className={cn(
                  "flex size-6 items-center justify-center rounded-r-sm hover:bg-hover-wash",
                  "disabled:pointer-events-none disabled:opacity-50",
                )}
              >
                <X className="size-3" aria-hidden="true" />
              </button>
            </span>
          </li>
        ))}
        <li>
          <Combobox
            value={inputValue}
            onValueChange={setInputValue}
            options={options}
            exactOption={exactOption}
            onSelect={(option) =>
              submitAdd({ id: Number(option.id), name: option.label })
            }
            createLabel={createLabel}
            onCreate={(spelling) => submitAdd({ name: spelling })}
            placeholder="タグを追加"
            icon={<Plus className="size-3 shrink-0 text-fg-muted" aria-hidden="true" />}
            busy={submitting}
            aria-label="タグを追加"
            inputRef={inputRef}
            // 一覧が閉じているときの Esc は、入力を空にする
            // （ui-design.md「Add input」）。
            onEscapeWhenClosed={() => setInputValue("")}
          />
        </li>
      </ul>
      {opError !== null && (
        <p role="alert" className="text-xs text-danger">
          {opError === "attach" ? "タグを付けられませんでした" : "タグを外せませんでした"}
        </p>
      )}
    </div>
  );
}

function buildOptions(
  allTags: readonly Tag[],
  attachedIds: ReadonlySet<number>,
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
    .filter((tag) => !attachedIds.has(tag.id))
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
