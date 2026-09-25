import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { errorMessage, RequestFailed } from "../api/client";
import { compareNatural } from "../api/tagOrder";
import { mergeTag, type Tag } from "../api/tags";
import Button from "../ui/Button";
import Combobox, { type ComboboxOption } from "../ui/Combobox";
import { ModalFrame } from "../ui/ModalFrame";

/**
 * MergeTagDialog は「別のタグへ統合…」の確認の窓である（ui-design.md「Merge
 * and delete」、受け入れ条件 12）。統合元（`source`）はその行のタグで固定、
 * 統合先を Combobox（統合元を除く全タグ、作成の行なし）で選ぶ。
 */
export default function MergeTagDialog({
  source,
  tags,
  onClose,
  onMerged,
  onStale,
}: {
  source: Tag;
  /** 統合先の候補を作る、共有のタグの一覧（統合元自身を除いて渡す前でよい）。 */
  tags: readonly Tag[];
  onClose: () => void;
  /** 統合が成功したときに呼ぶ。統合先の最新の状態を渡す。 */
  onMerged: (merged: Tag) => void;
  /** 統合元・統合先のどちらかがもう無い（tag_not_found）ときに呼ぶ。 */
  onStale: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  const mergeButton = useRef<HTMLButtonElement>(null);
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Tag | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // 選んだ直後（フォーカスをまだ動かしていない）かを持つ。統合先を選ぶと
  // 「統合する」へフォーカスを移すが、それは候補を選んだ直後の1回だけで、
  // 統合先を選び直した（別の候補、または綴りの完全一致）ときにも1回だけ
  // 動かす（B: フォーカスは常に動く意味のある要素へ。候補の一覧を開いたまま
  // 確認の文言に重ねない）。
  const justSelectedRef = useRef(false);

  const { options, exactOption } = buildTargetOptions(tags, source.id, value);

  useEffect(() => {
    if (justSelectedRef.current && target !== null) {
      justSelectedRef.current = false;
      mergeButton.current?.focus();
    }
  }, [target]);

  // 統合が失敗した直後は、まだ有効な操作（統合先を選んだままなら「統合する」、
  // そうでなければ「キャンセル」）へフォーカスを戻す。何もしないと、失敗の
  // 直前に disabled にした「統合する」がフォーカスを失い、body に落ちる
  // （N4）。
  useEffect(() => {
    if (error !== null) {
      (target !== null ? mergeButton : cancel).current?.focus();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [error]);

  /**
   * handleClose は窓を閉じるすべての経路（×・キャンセル・Esc）が通る。
   * 統合の要求が届いている間は、その応答がもう無いこの窓の状態を書き換える
   * ことを防ぐため閉じない（cancelDelete と同じ扱い。N3）。
   */
  function handleClose() {
    if (pending) return;
    onClose();
  }

  function selectTarget(option: ComboboxOption) {
    const found = tags.find((t) => String(t.id) === option.id);
    if (found === undefined) return;
    justSelectedRef.current = true;
    setTarget(found);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || pending) return;
    setError(null);
    setPending(true);
    try {
      const merged = await mergeTag(target.id, source.id);
      onMerged(merged);
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.code === "tag_not_found") {
        onStale();
        return;
      }
      setError(errorMessage(failure));
      setPending(false);
    }
  }

  return (
    <ModalFrame
      title={`「${source.name}」を統合`}
      onClose={handleClose}
      initialFocus={cancel}
    >
      {/*
        候補の一覧（Combobox）は overflow-y-auto の外に置く。中に置くと、
        窓の中身がまだ短い（確認の文言が出る前）うちは、この div 自身の
        高さも短く、候補の一覧（最大8行）が overflow-y-auto によって
        そこで切り取られてしまう（B3）。確認の文言・失敗の行だけを別の
        小さな overflow-y-auto に包み、長い文言でもそちらだけが縦に
        スクロールする。
      */}
      <div className="flex min-h-0 flex-1 flex-col gap-3 p-4 sm:p-5">
        <Combobox
          value={value}
          onValueChange={(next) => {
            setValue(next);
            setTarget(null);
            setError(null);
          }}
          options={options}
          exactOption={exactOption}
          onSelect={selectTarget}
          placeholder="統合先のタグ"
          aria-label="統合先のタグ"
          // 候補の一覧が閉じているときの Esc は、この窓を閉じる
          // （B1。一覧が開いていれば Combobox 自身が一覧だけを閉じ、
          // preventDefault するので ModalFrame の Esc には届かない）。
          onEscapeWhenClosed={handleClose}
          className="w-full"
        />
        <div className="min-h-0 flex-1 overflow-y-auto">
          {target !== null && (
            <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
              {`「${source.name}」が付いた ${String(source.videoCount)} 本の動画に「${target.name}」が付きます。「${source.name}」とそのシノニムは「${target.name}」のシノニムになり、「${source.name}」はタグの一覧から消えます。この操作は取り消せません。`}
            </p>
          )}
          {error !== null && (
            <p role="alert" className="mt-3 text-sm text-danger">
              {error}
            </p>
          )}
        </div>
      </div>
      <div className="flex shrink-0 justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={handleClose} disabled={pending}>
          キャンセル
        </Button>
        <Button
          ref={mergeButton}
          variant="danger"
          onClick={() => void submit()}
          disabled={pending || target === null}
        >
          {pending && <LoaderCircle className="animate-spin" />}
          統合する
        </Button>
      </div>
    </ModalFrame>
  );
}

function buildTargetOptions(
  tags: readonly Tag[],
  excludeId: number,
  input: string,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const trimmed = input.trim();
  const query = trimmed.toLowerCase();

  let exactTag: Tag | undefined;
  for (const tag of tags) {
    if (tag.id === excludeId) continue;
    if (tag.name === trimmed || tag.synonyms.includes(trimmed)) {
      exactTag = tag;
      break;
    }
  }

  const matched = tags
    .filter((tag) => tag.id !== excludeId)
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
