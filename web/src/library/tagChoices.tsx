import { CircleDashed } from "lucide-react";

import type { Tag, VideoTagsSummary } from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import type { ComboboxOption } from "../ui/Combobox";

type VideoTagsSummaryItem = VideoTagsSummary["items"][number];

/**
 * TagChoiceText は「タグを付ける」の候補に添える文言の出どころである。選択バーは
 * `t.library.selection`、再生画面は `t.player.tags` の文言を使う。文言は呼ぶたびに
 * カタログから引く（疑似ロケールへの差し替えが葉だけを替えるため）。
 */
export type TagChoiceText = "selection" | "player";

function choiceText(text: TagChoiceText) {
  return text === "selection" ? t.library.selection : t.player.tags;
}

/** IndexedTag は候補の絞り込みのために一度だけ作る、タグごとの下ごしらえである。 */
interface IndexedTag {
  tag: Tag;
  lowerName: string;
  lowerSynonyms: readonly string[];
}

/**
 * TagChoiceIndex はタグの一覧ごとに一度だけ作る索引である。`sorted` は名前の
 * 自然順（`compareNatural`、同順は元の並びのまま）に並べたもの、`exact` は綴り
 * （名前・シノニム）から、元の並びで最初にその綴りを持つタグへの対応である。
 */
interface TagChoiceIndex {
  sorted: readonly IndexedTag[];
  exact: ReadonlyMap<string, Tag>;
}

// 一覧は取得のたびに新しい配列になる（tags.ts の保持）。同じ配列の間は索引を
// 使い回し、打鍵のたびに小文字化と並べ替えをやり直さない（issue 675）。
const indexCache = new WeakMap<readonly Tag[], TagChoiceIndex>();

function tagChoiceIndex(allTags: readonly Tag[]): TagChoiceIndex {
  const cached = indexCache.get(allTags);
  if (cached !== undefined) return cached;
  const exact = new Map<string, Tag>();
  const entries: IndexedTag[] = allTags.map((tag) => {
    if (!exact.has(tag.name)) exact.set(tag.name, tag);
    for (const synonym of tag.synonyms) {
      if (!exact.has(synonym)) exact.set(synonym, tag);
    }
    return {
      tag,
      lowerName: tag.name.toLowerCase(),
      lowerSynonyms: tag.synonyms.map((synonym) => synonym.toLowerCase()),
    };
  });
  // Array.prototype.sort は安定なので、自然順で同順のタグは元の並びのまま残る。
  entries.sort((a, b) => compareNatural(a.tag.name, b.tag.name));
  const index = { sorted: entries, exact };
  indexCache.set(allTags, index);
  return index;
}

/**
 * buildTagChoices は「タグを付ける」の候補（各行の右に本数）を作る。選択バーと
 * 再生画面が共有する（ui-design.md「Combobox」）。名前かシノニムに入力を
 * 大文字小文字を区別せず含むタグを、前方一致を先に、その中は名前の自然順に
 * 並べる。シノニムだけで当たった行にはそのシノニムを添える。`excludedIds` の
 * タグは候補から除くが、`exactOption`（綴りがそのまま一致するタグ）からは除かない。
 *
 * 並べ替えは一覧ごとに一度だけ行い（索引）、打鍵のたびには自然順の並びを
 * 前から1度なめて、前方一致の行とそれ以外を順を保ったまま分ける。
 */
export function buildTagChoices(
  allTags: readonly Tag[],
  input: string,
  { text, excludedIds }: { text: TagChoiceText; excludedIds?: ReadonlySet<number> },
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const strings = choiceText(text);
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();
  const index = tagChoiceIndex(allTags);

  const prefixed: ComboboxOption[] = [];
  const rest: ComboboxOption[] = [];
  for (const { tag, lowerName, lowerSynonyms } of index.sorted) {
    if (excludedIds?.has(tag.id)) continue;
    if (query === "") {
      prefixed.push({
        id: String(tag.id),
        label: tag.name,
        hint: undefined,
        meta: strings.videoCount(tag.videoCount),
      });
      continue;
    }
    const nameMatch = lowerName.includes(query);
    // 当たったシノニムは、元の並びで最初に入力を含むもの（前方一致の判定と添える
    // 文言の両方に使う）。
    let hitIndex = -1;
    for (let i = 0; i < lowerSynonyms.length; i++) {
      if (lowerSynonyms[i]!.includes(query)) {
        hitIndex = i;
        break;
      }
    }
    if (!nameMatch && hitIndex === -1) continue;
    const prefix =
      lowerName.startsWith(query) ||
      (hitIndex !== -1 && lowerSynonyms[hitIndex]!.startsWith(query));
    const option: ComboboxOption = {
      id: String(tag.id),
      label: tag.name,
      hint: !nameMatch ? strings.synonym(tag.synonyms[hitIndex]!) : undefined,
      meta: strings.videoCount(tag.videoCount),
    };
    (prefix ? prefixed : rest).push(option);
  }
  for (const option of rest) prefixed.push(option);

  const exactTag = index.exact.get(trimmed);
  const exactOption: ComboboxOption | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: strings.videoCount(exactTag.videoCount),
        };

  return { options: prefixed, exactOption };
}

/** buildAddOptions は選択バーの「タグを付ける」の候補（全タグ、各行の右に本数）を作る。 */
export function buildAddOptions(
  allTags: readonly Tag[],
  input: string,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  return buildTagChoices(allTags, input, { text: "selection" });
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
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  function toOption(item: VideoTagsSummaryItem): ComboboxOption {
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
