import { LoaderCircle, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { errorMessage, RequestFailed } from "../api/client";
import { addTagSynonym, refreshTags, removeTagSynonym, type Tag } from "../api/tags";
import Button from "../ui/Button";
import Chip from "../ui/Chip";
import { isComposingKeyEvent } from "../ui/Combobox";
import { ModalFrame } from "../ui/ModalFrame";
import { tagFieldError, useTagNameField, type TagFieldError } from "./tagNameField";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isMergeRequired(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_merge_required";
}

/** MergeConfirm は「シノニム登録に伴う統合」の確認である（要件6、受け入れ条件17）。 */
interface MergeConfirm {
  /** 登録しようとした、既存のタグ S の元の名前（≒入力していた綴り）。 */
  name: string;
  sourceId: number;
  videoCount: number;
  hasSynonyms: boolean;
}

/** FocusAfterRemoval は、解除したシノニムのチップが消えたあとの移動先を持つ。 */
type FocusAfterRemoval = {
  target: { name: string } | "input";
  /** 消えるのを待つシノニムの名前。無ければ即座に移す（失敗して戻すときなど）。 */
  removedName?: string;
};

/**
 * SynonymsDialog は「シノニム」で開く窓である（ui-design.md「Synonyms」）。
 * 上に今のシノニムを Chip の並びで、下に追加の入力を持つ。追加しようとした
 * 名前が既存のタグの名前と一致すると、同じ窓の中を統合の確認に切り替える
 * （要件6「シノニム登録」、受け入れ条件17）。
 */
export default function SynonymsDialog({
  tag,
  onClose,
  onTagUpdated,
  onStale,
}: {
  tag: Tag;
  onClose: () => void;
  /** 登録・解除・統合のどれかが成功したときに、タグの最新の状態を渡す。 */
  onTagUpdated: (tag: Tag) => void;
  /** このタグがもう無い（tag_not_found）ときに呼ぶ。 */
  onStale: () => void;
}) {
  const field = useTagNameField("", () => setAddError(null));
  const inputRef = useRef<HTMLInputElement | null>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const confirmMergeButton = useRef<HTMLButtonElement>(null);

  // tagRef は常に最新の tag（props）を指す。解除・統合の応答が届いた時点で
  // シノニムの一覧を作るときは、要求を送った時点で閉じ込めた（古いかもしれ
  // ない）`tag` ではなく、この ref 経由の最新の値を使う（N5）。
  const tagRef = useRef(tag);
  tagRef.current = tag;

  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [addPending, setAddPending] = useState(false);
  const [addError, setAddError] = useState<TagFieldError | null>(null);

  const [confirm, setConfirm] = useState<MergeConfirm | null>(null);
  const [confirmPending, setConfirmPending] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);

  const chipRefs = useRef(new Map<string, HTMLButtonElement>());
  const pendingFocusRef = useRef<FocusAfterRemoval | null>(null);

  // confirm が変わるたびに、そちらのビューの最初のフォーカス先へ移す（開いた
  // 直後の最初のフォーカスは ModalFrame の initialFocus に任せるので、ここでは
  // 実際に切り替わったときだけ動かす）。
  const mountedRef = useRef(false);
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true;
      return;
    }
    if (confirm !== null) backRef.current?.focus();
    else inputRef.current?.focus();
  }, [confirm]);

  // シノニムの解除が失敗した直後は「統合する」（または「戻る」）へ戻す
  // MergeTagDialog の N4 と同じ扱いを、統合の確認の「統合する」にも適用する。
  useEffect(() => {
    if (confirmError !== null) {
      (confirm !== null ? confirmMergeButton : backRef).current?.focus();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [confirmError]);

  // 解除したチップが一覧から消えたら（tag.synonyms に反映されたら）、次の
  // チップの ×、無ければ前のチップの ×、1つも無ければ入力へフォーカスを
  // 移す（N2）。外せなかったとき（pendingFocusRef が removedName を持たない
  // 形で積み直される）は、その場でそのチップ自身の × へ戻す。
  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending === null) return;
    if (pending.removedName !== undefined && tag.synonyms.includes(pending.removedName)) {
      return;
    }
    const target = pending.target;
    if (target !== "input" && removing.has(target.name)) return;
    pendingFocusRef.current = null;
    if (target === "input") {
      inputRef.current?.focus();
      return;
    }
    chipRefs.current.get(target.name)?.focus();
  }, [tag.synonyms, removing]);

  async function removeSynonym(name: string) {
    const order = tagRef.current.synonyms;
    const index = order.indexOf(name);
    const next = order[index + 1];
    const previous = index > 0 ? order[index - 1] : undefined;
    pendingFocusRef.current = {
      target:
        next !== undefined
          ? { name: next }
          : previous !== undefined
            ? { name: previous }
            : "input",
      removedName: name,
    };
    setRemoving((current) => new Set(current).add(name));
    setAddError(null);
    try {
      await removeTagSynonym(tag.id, name);
      const current = tagRef.current;
      onTagUpdated({ ...current, synonyms: current.synonyms.filter((s) => s !== name) });
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      // 外せなかったときは、次/前のチップではなく、このチップ自身の × へ戻す
      // （`web/src/player/VideoTags.tsx` の removeTag と同じ扱い）。
      pendingFocusRef.current = { target: { name } };
      setAddError({ kind: "other", message: errorMessage(failure) });
    } finally {
      setRemoving((current) => {
        const next = new Set(current);
        next.delete(name);
        return next;
      });
    }
  }

  /**
   * openConfirm は tag_merge_required を受けたときに呼ぶ。タグの一覧を取り直し
   * （別のタブでの変更を拾うため）、その名前を元の名前として持つタグ S を
   * 探して確認へ切り替える。契約（tags-api.md §3）どおり、承諾は S の id を
   * 添えて送るので、ここで正しい S を確かめ直す（確認の後に別のタブでその
   * 名前が移っていても、確認していないタグを統合しないため）。
   *
   * `attempt` は、この関数と `submitAdd` の間の自動の行き来（一覧を取り直した
   * ら名前の持ち主がもう無く、素の登録をやり直したらまた tag_merge_required
   * になる…という往復）を1回までに区切る（N6b）。それでも収まらないときは、
   * ループを続けず一般の失敗として見せる。
   */
  async function openConfirm(name: string, attempt = 0) {
    try {
      const list = await refreshTags();
      const found = list.find((item) => item.name === name);
      if (found === undefined) {
        if (attempt >= 1) {
          setAddError({ kind: "other", message: "タグを追加できませんでした" });
          return;
        }
        // 別のタブでの変更により、この名前を持つタグがもう無い。素の登録を
        // やり直せば通るはずなので、確認は出さずにもう一度試す。
        await submitAdd(name, attempt + 1);
        return;
      }
      setConfirm({
        name,
        sourceId: found.id,
        videoCount: found.videoCount,
        hasSynonyms: found.synonyms.length > 0,
      });
    } catch (failure) {
      setAddError({ kind: "other", message: errorMessage(failure) });
    }
  }

  async function submitAdd(spelling?: string, attempt = 0) {
    const name = spelling ?? field.trySpelling();
    if (name === null) return;
    setAddError(null);
    setAddPending(true);
    try {
      const updated = await addTagSynonym(tag.id, name);
      onTagUpdated(updated);
      field.setValue("");
      // N6a: openConfirm の素の登録のやり直しが成功したときも含め、確認の
      // ビューを出したままにしない。
      setConfirm(null);
      setAddPending(false);
    } catch (failure) {
      // 確認（統合）のビューへ切り替わる前に、いま出している入力の
      // aria-busy を必ず下ろす。合流先の await の間そのままにすると、
      // 入力が確認のビューに差し替わって消えるその瞬間の見た目に残る。
      setAddPending(false);
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      if (isMergeRequired(failure)) {
        if (attempt >= 1) {
          setAddError({ kind: "other", message: "タグを追加できませんでした" });
          return;
        }
        await openConfirm(name, attempt + 1);
        return;
      }
      setAddError(tagFieldError(failure));
    }
  }

  function backFromConfirm() {
    if (confirmPending) return;
    setConfirm(null);
    setConfirmError(null);
  }

  async function acceptMerge() {
    if (confirm === null) return;
    setConfirmError(null);
    setConfirmPending(true);
    try {
      const updated = await addTagSynonym(tag.id, confirm.name, confirm.sourceId);
      onTagUpdated(updated);
      setConfirm(null);
      field.setValue("");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      if (isMergeRequired(failure)) {
        // 確認の後に別のタブでこの名前がさらに別のタグへ移った。一覧を
        // 取り直して確認をやり直す（tags-api.md §3）。
        await openConfirm(confirm.name);
        return;
      }
      setConfirmError(errorMessage(failure));
    } finally {
      setConfirmPending(false);
    }
  }

  /**
   * handleClose は窓を閉じるすべての経路（×・Esc）が通る。統合の要求が
   * 届いている間は、その応答がもう無いこの窓の状態を書き換えることを
   * 防ぐため閉じない（cancelDelete・MergeTagDialog と同じ扱い。N3）。
   */
  function handleClose() {
    if (confirmPending) return;
    onClose();
  }

  const reasonId = "synonym-add-reason";
  const errorId = "synonym-add-error";

  return (
    <ModalFrame
      title={`「${tag.name}」のシノニム`}
      onClose={handleClose}
      initialFocus={inputRef}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 sm:p-5">
        {confirm === null ? (
          <>
            {tag.synonyms.length > 0 && (
              <ul aria-label="シノニム" className="flex flex-wrap gap-1.5">
                {tag.synonyms.map((name) => (
                  <li key={name} className="min-w-0 max-w-full">
                    <Chip
                      tone="onElevated"
                      title={name}
                      className="max-w-full gap-0.5 pr-1"
                    >
                      <span className="min-w-0 truncate">{name}</span>
                      <button
                        ref={(node) => {
                          if (node) chipRefs.current.set(name, node);
                          else chipRefs.current.delete(name);
                        }}
                        type="button"
                        aria-label={`シノニム「${name}」を解除`}
                        aria-busy={removing.has(name) || undefined}
                        onClick={() => {
                          if (removing.has(name)) return;
                          void removeSynonym(name);
                        }}
                        className="flex size-4 shrink-0 items-center justify-center rounded-sm hover:bg-hover-wash disabled:opacity-50"
                      >
                        <X className="size-3" aria-hidden="true" />
                      </button>
                    </Chip>
                  </li>
                ))}
              </ul>
            )}

            <div className="flex items-end gap-2">
              <div className="min-w-0 flex-1">
                <label htmlFor="synonym-add-input" className="sr-only">
                  シノニムを追加
                </label>
                <input
                  id="synonym-add-input"
                  ref={inputRef}
                  value={field.value}
                  onChange={(event) => field.setValue(event.target.value)}
                  onPaste={field.onPaste}
                  onBeforeInput={field.onBeforeInput}
                  onKeyDown={(event) => {
                    if (isComposingKeyEvent(event)) return;
                    if (event.key === "Enter") {
                      event.preventDefault();
                      if (!addPending) void submitAdd();
                    }
                  }}
                  placeholder="シノニムを追加"
                  aria-label="シノニムを追加"
                  aria-describedby={
                    field.reason !== null
                      ? reasonId
                      : addError !== null
                        ? errorId
                        : undefined
                  }
                  aria-busy={addPending || undefined}
                  className="h-8 w-full min-w-0 rounded-sm border border-border bg-field px-2 text-sm text-fg focus:border-accent focus:outline-none"
                />
              </div>
              <Button
                onClick={() => {
                  if (!addPending) void submitAdd();
                }}
              >
                追加
              </Button>
            </div>
            {field.reason !== null && (
              <p id={reasonId} className="-mt-2 text-xs text-danger">
                {field.reason}
              </p>
            )}
            {field.reason === null && addError !== null && addError.kind === "taken" && (
              <p id={errorId} className="-mt-2 text-xs text-danger">
                {addError.message}
              </p>
            )}
            {field.reason === null && addError !== null && addError.kind === "other" && (
              <p id={errorId} role="alert" className="-mt-2 text-sm text-danger">
                {addError.message}
              </p>
            )}
          </>
        ) : (
          <>
            <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
              {`「${confirm.name}」は ${String(confirm.videoCount)} 本の動画に付いているタグです。「${tag.name}」に統合すると、その ${String(confirm.videoCount)} 本に「${tag.name}」が付き、「${confirm.name}」${confirm.hasSynonyms ? "とそのシノニムは" : "は"}「${tag.name}」のシノニムになります。「${confirm.name}」はタグの一覧から消えます。`}
            </p>
            {confirmError !== null && (
              <p role="alert" className="text-sm text-danger">
                {confirmError}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <Button ref={backRef} onClick={backFromConfirm} disabled={confirmPending}>
                戻る
              </Button>
              <Button
                ref={confirmMergeButton}
                variant="danger"
                onClick={() => void acceptMerge()}
                disabled={confirmPending}
              >
                {confirmPending && <LoaderCircle className="animate-spin" />}
                統合する
              </Button>
            </div>
          </>
        )}
      </div>
    </ModalFrame>
  );
}
