import { CircleDashed } from "lucide-react";

import type { Tag, VideoTagsSummary } from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import type { TagChoice } from "./TagCommand";

type VideoTagsSummaryItem = VideoTagsSummary["items"][number];

/** buildAddOptions は「タグを付ける」の候補（全タグ、各行の右に本数）を作る。 */
export function buildAddOptions(
  allTags: readonly Tag[],
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
            ? t.library.selection.synonym(synonymHit)
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
    meta: t.library.selection.videoCount(tag.videoCount),
  }));

  const exactOption: TagChoice | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.library.selection.videoCount(exactTag.videoCount),
        };

  return { options, exactOption };
}

/**
 * removableSummary は要約から、選んだ動画のどれかに手で付けたタグ
 * （`manualCount >= 1`）だけを残す。フォルダ名から付いているだけのタグは
 * 外せないので、候補にも `disabled` の行にも出さない
 * （specs/017-folder-groups/ui-design.md「Folder-derived tag chip」）。
 */
export function removableSummary(summary: VideoTagsSummary): VideoTagsSummary {
  return { ...summary, items: summary.items.filter((item) => item.manualCount >= 1) };
}

/**
 * buildRemoveOptions は「タグを外す」の候補（要約のタグだけ）を作る。作成の行は持たない。
 * 外れるのは手で付けた分だけなので、「一部」の判定と本数は `manualCount` で行い、
 * 分母は選んだ本数（`total`）のままにする。
 */
export function buildRemoveOptions(
  summary: VideoTagsSummary,
  input: string,
): { options: TagChoice[]; exactOption: TagChoice | null } {
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  function toOption(item: VideoTagsSummaryItem): TagChoice {
    const partial = item.manualCount < summary.total;
    return {
      id: String(item.tag.id),
      label: item.tag.name,
      meta: partial ? (
        <span className="inline-flex items-center gap-1">
          <CircleDashed className="size-3" aria-hidden="true" />
          <span>{t.library.selection.partial(item.manualCount, summary.total)}</span>
        </span>
      ) : (
        t.library.selection.videoCount(summary.total)
      ),
      ariaLabel: partial
        ? t.library.selection.partialLabel(item.tag.name, item.manualCount, summary.total)
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
