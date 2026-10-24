import { Check, Folder, Plus, X } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  confirmVideoTag,
  rejectVideoTag,
  RequestFailed,
  type TagRef,
  type VideoTag,
} from "../api/client";
import {
  attachVideoTagByID,
  attachVideoTagByName,
  currentTags,
  detachVideoTag,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import {
  applyTagToTags,
  compareNatural,
  isFolderOnly,
  showsTentative,
  tagsReflectChange,
} from "../api/tagOrder";
import { subscribeVideoTags } from "../api/videoTagsEvents";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import TagCommand, { type TagChoice } from "../ui/TagCommand";
import TentativeMark from "../ui/TentativeMark";
import { useToast } from "../ui/Toast";

export default function VideoTags({
  videoId,
  tags: initialTags,
  onStaleVideo,
  onChanged,
}: {
  videoId: number;
  tags: readonly VideoTag[];
  onStaleVideo: () => void;
  onChanged?: () => void;
}) {
  const toast = useToast();

  // Overlay confirmed tag changes until refreshed video data includes them, preventing stale refreshes from erasing edits.
  //
  // Clear overlays on video changes even when the component is reused without a key.
  //
  // Compare removals only against tag lists received after the change; earlier lists may simply lack a newly created tag.
  const appliedRef = useRef(
    new Map<number, { tag: TagRef; action: "add" | "remove"; asOfSeq: number }>(),
  );
  const prevVideoIdRef = useRef(videoId);
  const [tags, setTags] = useState<readonly VideoTag[]>(initialTags);

  // Revalidate on mount, sharing recent or pending requests and showing cached tags while waiting.
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  // Count completed live fetches only; the initial cache may predate local edits.
  const tagsFetchSeqRef = useRef(0);
  useEffect(() => {
    let alive = true;
    revalidateTags()
      .then((loaded) => {
        if (!alive) return;
        tagsFetchSeqRef.current += 1;
        setAllTags(loaded);
      })
      .catch(() => undefined);
    const unsubscribe = subscribeTags((loaded) => {
      if (!alive) return;
      tagsFetchSeqRef.current += 1;
      setAllTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    const applied = appliedRef.current;
    if (prevVideoIdRef.current !== videoId) {
      prevVideoIdRef.current = videoId;
      applied.clear();
      setTags(initialTags);
      return;
    }
    let next = initialTags;
    for (const [tagId, change] of applied) {
      if (tagsReflectChange(initialTags, tagId, change.action)) {
        applied.delete(tagId);
        continue;
      }
      // A later fetched list can prove a tag was deleted or merged; otherwise retain the local overlay.
      if (
        allTags !== undefined &&
        tagsFetchSeqRef.current > change.asOfSeq &&
        !allTags.some((tag) => tag.id === tagId)
      ) {
        applied.delete(tagId);
        continue;
      }
      next = applyTagToTags(next, change.tag, change.action);
    }
    setTags(next);
  }, [initialTags, videoId, allTags]);
  useEffect(
    () =>
      subscribeVideoTags((videoIds, tag, action) => {
        if (!videoIds.includes(videoId)) return;
        appliedRef.current.set(tag.id, {
          tag,
          action,
          asOfSeq: tagsFetchSeqRef.current,
        });
        setTags((current) => applyTagToTags(current, tag, action));
      }),
    [videoId],
  );

  const [inputValue, setInputValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [removingIds, setRemovingIds] = useState<ReadonlySet<number>>(new Set());
  const [opError, setOpError] = useState<UiText | null>(null);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const buttonRefs = useRef(new Map<number, HTMLButtonElement>());
  // Keep focus on the removing chip until the server confirms removal.
  const pendingFocusRef = useRef<{
    target: { chipId: number } | "input";
    removedId?: number;
  } | null>(null);

  // Move focus during layout to avoid a transient body focus visible to assistive technology.
  useLayoutEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending === null) return;
    // Folder-derived chips remain after manual removal; wait for the manual assignment, not DOM removal.
    if (
      pending.removedId !== undefined &&
      !tagsReflectChange(tags, pending.removedId, "remove")
    ) {
      return;
    }
    // After failure, wait until this chip's remove button is enabled before restoring focus.
    const target = pending.target;
    if (target !== "input" && removingIds.has(target.chipId)) return;
    pendingFocusRef.current = null;
    if (target === "input") {
      inputRef.current?.focus();
      return;
    }
    buttonRefs.current.get(target.chipId)?.focus();
  }, [tags, removingIds]);

  // Folder-only tags remain selectable because users can also assign them manually.
  const attachedIds = new Set(tags.filter((tag) => tag.manual).map((tag) => tag.id));
  const { options, exactOption } = buildOptions(allTags ?? [], attachedIds, inputValue);

  function isTagNotFound(error: unknown): boolean {
    return error instanceof RequestFailed && error.code === "tag_not_found";
  }

  function submitAdd(tag: { id: number; name: string } | { name: string }) {
    setOpError(null);
    setSubmitting(true);
    const displayName = tag.name;
    // Preserve a new name entered while the previous request was pending.
    const submittedValue = inputValue;
    const request =
      "id" in tag
        ? attachVideoTagByID([videoId], tag.id)
        : attachVideoTagByName([videoId], tag.name);
    void request
      .then(() => {
        setInputValue((current) => (current === submittedValue ? "" : current));
        onChanged?.();
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(t.player.tags.gone(displayName));
          onStaleVideo();
          return;
        }
        setOpError(t.player.tags.attachFailed(errorText(error)));
      })
      .finally(() => setSubmitting(false));
  }

  // Apply confirmation/rejection immediately after success, then refresh video data.
  function settleTag(tag: VideoTag, action: "confirm" | "reject") {
    setOpError(null);
    setRemovingIds((current) => new Set(current).add(tag.id));
    const settle = action === "confirm" ? confirmVideoTag : rejectVideoTag;
    void settle(videoId, tag.id)
      .then(() => {
        setTags((current) =>
          action === "confirm"
            ? current.map((item) =>
                item.id === tag.id ? { ...item, unconfirmed: false } : item,
              )
            : current.filter((item) => item.id !== tag.id),
        );
        onChanged?.();
      })
      .catch((error: unknown) => {
        if (error instanceof RequestFailed && error.status === 404) {
          onStaleVideo();
          return;
        }
        setOpError(t.player.tags.settleFailed(errorText(error)));
      })
      .finally(() => {
        setRemovingIds((current) => {
          const next = new Set(current);
          next.delete(tag.id);
          return next;
        });
      });
  }

  function removeTag(tag: TagRef, index: number) {
    // Focus only manually assigned chips with remove buttons; fall back next, previous, then input.
    const next =
      tags.slice(index + 1).find((candidate) => candidate.manual) ??
      tags
        .slice(0, index)
        .reverse()
        .find((candidate) => candidate.manual);
    pendingFocusRef.current = {
      target: next !== undefined ? { chipId: next.id } : "input",
      removedId: tag.id,
    };
    setOpError(null);
    setRemovingIds((current) => new Set(current).add(tag.id));
    void detachVideoTag([videoId], tag.id)
      .then(() => onChanged?.())
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(t.player.tags.gone(tag.name));
          onStaleVideo();
          return;
        }
        pendingFocusRef.current = { target: { chipId: tag.id } };
        setOpError(t.player.tags.detachFailed(errorText(error)));
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
        <Plus className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{t.player.tags.create(trimmed)}</span>
      </span>
    ) : null;

  return (
    <div className="flex flex-col gap-1">
      <h2 className="sr-only">{t.player.tags.heading}</h2>
      <ul className="flex flex-wrap items-center gap-1.5">
        {tags.map((tag, index) => (
          <li key={tag.id} className="min-w-0 max-w-full">
            {isFolderOnly(tag) ? (
              // Folder-derived assignments cannot be removed per video.
              <Badge
                asChild
                variant="outline"
                className="max-w-full justify-start border-dashed border-input font-normal hover:border-solid"
              >
                <Link
                  to={libraryFilterPath(TAG_PARAM, tag.id)}
                  title={tag.name}
                  aria-label={
                    tag.tentative
                      ? t.player.tags.filterByFromFolderTentative(tag.name)
                      : t.player.tags.filterByFromFolder(tag.name)
                  }
                >
                  <Folder className="text-muted-foreground" aria-hidden="true" />
                  <span className="min-w-0 truncate">{tag.name}</span>
                  {tag.tentative && <TentativeMark />}
                </Link>
              </Badge>
            ) : (
              <Badge
                variant="secondary"
                title={tag.name}
                className="max-w-full justify-start gap-0 pr-0 font-normal"
              >
                {/* Truncate only the name so the tentative marker remains visible. */}
                <Link
                  to={libraryFilterPath(TAG_PARAM, tag.id)}
                  aria-label={
                    showsTentative(tag)
                      ? t.player.tags.filterByTentative(tag.name)
                      : t.player.tags.filterBy(tag.name)
                  }
                  className={cn(
                    "min-w-0 hover:text-primary",
                    showsTentative(tag) ? "inline-flex items-center gap-1" : "truncate",
                  )}
                >
                  {showsTentative(tag) ? (
                    <>
                      <span className="min-w-0 truncate">{tag.name}</span>
                      <TentativeMark />
                    </>
                  ) : (
                    tag.name
                  )}
                </Link>
                <span aria-hidden="true" className="ml-1.5 h-4 w-px shrink-0 bg-input" />
                {tag.unconfirmed === true ? (
                  <>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t.player.tags.confirm(tag.name)}
                      title={t.player.tags.confirm(tag.name)}
                      disabled={removingIds.has(tag.id)}
                      onClick={() => settleTag(tag, "confirm")}
                      className="size-6 rounded-none [&_svg]:size-3"
                    >
                      <Check aria-hidden="true" />
                    </Button>
                    <Button
                      ref={(node) => {
                        if (node) buttonRefs.current.set(tag.id, node);
                        else buttonRefs.current.delete(tag.id);
                      }}
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t.player.tags.reject(tag.name)}
                      title={t.player.tags.reject(tag.name)}
                      disabled={removingIds.has(tag.id)}
                      onClick={() => settleTag(tag, "reject")}
                      className="size-6 rounded-l-none rounded-r-sm [&_svg]:size-3"
                    >
                      <X aria-hidden="true" />
                    </Button>
                  </>
                ) : (
                  <Button
                    ref={(node) => {
                      if (node) buttonRefs.current.set(tag.id, node);
                      else buttonRefs.current.delete(tag.id);
                    }}
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t.player.tags.remove(tag.name)}
                    disabled={removingIds.has(tag.id)}
                    onClick={() => removeTag(tag, index)}
                    className="size-6 rounded-l-none rounded-r-sm [&_svg]:size-3"
                  >
                    <X aria-hidden="true" />
                  </Button>
                )}
              </Badge>
            )}
          </li>
        ))}
        {/* Without tags, pull the quiet field's inner padding back so its text lines up with the title. */}
        <li className="first:-ml-1.5">
          <TagCommand
            layout="dropdown"
            label={t.player.tags.add}
            // A quiet "+ Add tag" until pointed at or focused.
            frameClassName="w-combobox border-transparent bg-transparent hover:border-input focus-within:border-input focus-within:bg-muted"
            value={inputValue}
            onValueChange={setInputValue}
            choices={options}
            exactChoice={exactOption}
            onSelect={(option) =>
              submitAdd({ id: Number(option.id), name: option.label })
            }
            createLabel={createLabel}
            onCreate={(spelling) => submitAdd({ name: spelling })}
            icon={
              <Plus
                className="size-3 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            }
            busy={submitting}
            inputRef={inputRef}
            //
            onEscape={() => setInputValue("")}
          />
        </li>
      </ul>
      {opError !== null && (
        <p role="alert" className="text-xs text-destructive">
          {opError}
        </p>
      )}
    </div>
  );
}

function buildOptions(
  allTags: readonly Tag[],
  attachedIds: ReadonlySet<number>,
  input: string,
): { options: TagChoice[]; exactOption: TagChoice | null } {
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
          !nameMatch && synonymHit !== undefined
            ? t.player.tags.synonym(synonymHit)
            : undefined,
      };
    })
    .filter((value): value is NonNullable<typeof value> => value !== null)
    .sort((a, b) => {
      if (a.prefix !== b.prefix) return a.prefix ? -1 : 1;
      return compareNatural(a.tag.name, b.tag.name);
    });

  const options: TagChoice[] = matched.map(({ tag, hint }) => ({
    id: String(tag.id),
    label: tag.name,
    hint,
    meta: t.player.tags.videoCount(tag.videoCount),
  }));

  const exactOption: TagChoice | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.player.tags.videoCount(exactTag.videoCount),
        };

  return { options, exactOption };
}
