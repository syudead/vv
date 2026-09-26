import { Folder, Plus, X } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { RequestFailed, type TagRef, type VideoTag } from "../api/client";
import {
  attachVideoTagByID,
  attachVideoTagByName,
  currentTags,
  detachVideoTag,
  refreshTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import {
  applyTagToTags,
  compareNatural,
  isFolderOnly,
  tagsReflectChange,
} from "../api/tagOrder";
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
  tags: readonly VideoTag[];
  /** タグがもう無い（tag_not_found）ときに、この動画を取り直すために呼ぶ。 */
  onStaleVideo: () => void;
}) {
  const toast = useToast();

  // この画面で確定した付け外しを、動画の情報に重ねて持つ。付け外しの前に
  // 始まった動画の取り直しが後から届いても、確定したタグを消さない。
  // 届いた情報がその付け外しをすでに映していれば、重ねる必要は無いので捨てる。
  //
  // VideoPage は videoId ごとに VideoTags を作り直さず使い回すことがあるため
  // （key を付けない再利用。Devin の指摘1）、videoId が変われば、重ねた分を
  // 前の動画に残さず必ず捨てる。
  //
  // 重ねた各エントリは、記録した時点の tagsFetchSeqRef（下）の値も
  // `asOfSeq` として持つ。allTags でその付与を蘇らせない判定（下）に使うのは
  // 「その付与を記録した後に実際に届いた」allTags に限る。記録する前から
  // 持っていた allTags（作ったばかりのタグがまだそこに無いだけ）で判定すると、
  // 別画面での削除と誤認してしまう（Devin の指摘）。
  const appliedRef = useRef(
    new Map<number, { tag: TagRef; action: "add" | "remove"; asOfSeq: number }>(),
  );
  const prevVideoIdRef = useRef(videoId);
  const [tags, setTags] = useState<readonly VideoTag[]>(initialTags);

  // 画面が開くときは、共有の保持がすでにあっても必ず取り直す（plan の
  // Structural Decisions 8「画面が開くとき…に refreshTags で取り直す」）。
  // 取り直す間は、あれば直近の保持を初期値として先に出す。
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  // tagsFetchSeqRef は、実際に届いた（=取得が生きたまま tags.ts の held を
  // 更新した）タグの一覧の回数を数える。マウント時点の allTags の初期値
  // （currentTags()。今の付け外しより前に取れていたかもしれない）はこれに
  // 数えない。
  const tagsFetchSeqRef = useRef(0);
  useEffect(() => {
    let alive = true;
    refreshTags()
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
      // 動画が替わった（VideoTags を使い回した）。前の動画の重ねを持ち越さない。
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
      // 重ねたタグ自体が共有の一覧からもう消えていれば（別画面での削除・
      // 統合）、その付与を蘇らせない（Devin の指摘1）。ただし、これを判定
      // できるのは、その付与を記録した後に実際に取得が届いていて
      // （tagsFetchSeqRef が進んでいて）、かつ allTags を取得済み
      // （undefined でない）ときだけ。どちらか欠ければ確かめようが無いので、
      // 今までどおり重ねる。
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
  const [opError, setOpError] = useState<"attach" | "detach" | null>(null);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const buttonRefs = useRef(new Map<number, HTMLButtonElement>());
  // removedId は、外すのを待っているチップ。そのチップの手で付けた分が外れる
  // （サーバーが外したと応えた）まで、フォーカスを動かさない。
  const pendingFocusRef = useRef<{
    target: { chipId: number } | "input";
    removedId?: number;
  } | null>(null);

  // フォーカスは、チップが消えた（または押せるようになった）のと同じ描画の中で
  // 移す。useEffect だと描画から効果までの間にフォーカスが body に落ち、
  // その間に DOM を読んだ側（支援技術やテスト）には行き先が見えない。
  useLayoutEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending === null) return;
    // 外すのは手で付けた分だけで、フォルダ名からも付いているチップは残る
    // （017 の contracts/folder-groups-api.md §4）。待つのは手で付けた分が
    // 外れるまでで、チップが消えるまでではない。
    if (
      pending.removedId !== undefined &&
      !tagsReflectChange(tags, pending.removedId, "remove")
    ) {
      return;
    }
    // 外せなかったときの戻し先は、まだ disabled のままの、そのチップ自身の ×
    // かもしれない。再び押せるようになるまで（removingIds から消えるまで）待つ。
    const target = pending.target;
    if (target !== "input" && removingIds.has(target.chipId)) return;
    pendingFocusRef.current = null;
    if (target === "input") {
      inputRef.current?.focus();
      return;
    }
    buttonRefs.current.get(target.chipId)?.focus();
  }, [tags, removingIds]);

  // 候補から除くのは手で付けたタグだけ。フォルダ名からだけ付いているタグは
  // 候補に出し、確定すると手でも付いて面のある形に変わる（017 の ui-design.md
  // 「Folder-derived tag chip」）。
  const attachedIds = new Set(tags.filter((tag) => tag.manual).map((tag) => tag.id));
  const { options, exactOption } = buildOptions(allTags ?? [], attachedIds, inputValue);

  function isTagNotFound(error: unknown): boolean {
    return error instanceof RequestFailed && error.code === "tag_not_found";
  }

  function submitAdd(tag: { id: number; name: string } | { name: string }) {
    setOpError(null);
    setSubmitting(true);
    const displayName = tag.name;
    // 応答を待つ間に次の名前を打っていたら、それは消さない。
    const submittedValue = inputValue;
    const request =
      "id" in tag
        ? attachVideoTagByID([videoId], tag.id)
        : attachVideoTagByName([videoId], tag.name);
    void request
      .then(() => setInputValue((current) => (current === submittedValue ? "" : current)))
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
    // 移す先は × を持つ（手で付けた）チップだけ。フォルダ名からだけ付いている
    // 破線のチップは × を持たないので飛ばす（017 の ui-design.md
    // 「Folder-derived tag chip」、014 と同じ次 → 前 → 入力の規則）。
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
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(`タグ「${tag.name}」はもう無いため、一覧を取り直しました`);
          onStaleVideo();
          return;
        }
        // 外せなかったときは、次/前のチップではなく、このチップの × へ戻す。
        pendingFocusRef.current = { target: { chipId: tag.id } };
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
            {isFolderOnly(tag) ? (
              // フォルダ名からだけ付いているタグは動画ごとには外せないので、×
              // を出さず、破線の枠と Folder の目印で出す。大きさ（h-6・text-xs）と
              // 文字の色は面のあるチップと同じ（017 の ui-design.md
              // 「Folder-derived tag chip」「Interaction states」）。
              <Link
                to={`/?tag=${String(tag.id)}`}
                title={tag.name}
                aria-label={`${tag.name}で絞り込む（フォルダ名から）`}
                className="inline-flex h-6 max-w-full items-center gap-1 rounded-sm border border-dashed border-border-strong px-2 text-xs text-fg hover:border-solid hover:text-fg"
              >
                <Folder className="size-3 shrink-0 text-fg-subtle" aria-hidden="true" />
                <span className="min-w-0 truncate">{tag.name}</span>
              </Link>
            ) : (
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
            )}
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
