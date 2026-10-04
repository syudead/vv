import { X } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { addTagSynonym, refreshTags, removeTagSynonym, type Tag } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { isComposingKeyEvent, isComposingNativeKeyEvent } from "../ui/Combobox";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/shadcn/dialog";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { DialogError } from "./DialogError";
import { tagFieldError, useTagNameField, type TagFieldError } from "./tagNameField";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isMergeRequired(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_merge_required";
}

/** mergeOwner は tag_merge_required が返した、名前を持つタグの元の名前である。 */
function mergeOwner(error: unknown): string | undefined {
  return error instanceof RequestFailed ? error.tagName : undefined;
}

/** MergeConfirm は「シノニム登録に伴う統合」の確認である（要件6、受け入れ条件17）。 */
interface MergeConfirm {
  /** 登録しようとした、既存のタグ S の元の名前（≒入力していた綴り）。 */
  name: string;
  /** タグ S の元の名前（API の tagName。確認の文言に出す）。 */
  sourceName: string;
  sourceId: number;
  /**
   * 確かめ直したときのタグ S（統合の前の状態）。統合したら呼び出し元へ渡し、読み込んで
   * いない S でも今の条件に合っていたかで件数を数え直せるようにする（TagsPage）。
   */
  source: Tag;
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
  onSynonymRemoved,
  onStale,
}: {
  tag: Tag;
  onClose: () => void;
  /**
   * 登録・シノニム登録に伴う統合が成功したときに、タグの最新の状態を渡す。
   * `removed` は統合元の統合の前の状態で、統合元がタグの一覧から消えたことを
   * 呼び出し元へ伝える（渡さなければ何も消えていない。TagsPage 参照）。
   */
  onTagUpdated: (tag: Tag, removed?: Tag) => void;
  /**
   * シノニムの解除が成功したときに呼ぶ。呼び出し元は、この呼び出し時点の
   * 最新の一覧からその名前だけを取り除く（`tag` prop の閉じ込めではなく）。
   * ほぼ同時に複数のシノニムを解除したとき、互いの結果を巻き戻さないため
   * （N5）。
   */
  onSynonymRemoved: (tagId: number, name: string) => void;
  /** このタグがもう無い（tag_not_found）ときに呼ぶ。 */
  onStale: () => void;
}) {
  const field = useTagNameField("", () => setAddError(null));
  const inputRef = useRef<HTMLInputElement | null>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const confirmMergeButton = useRef<HTMLButtonElement>(null);

  // fieldValueRef は常に最新の入力値を指す。応答が届いた時点で、送信した
  // ときの値のままなら入力を空にするが、その間に打ち直していれば残す
  // （N: 送信中に打ち直した名前を消さない）。
  const fieldValueRef = useRef(field.value);
  fieldValueRef.current = field.value;

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
  const [confirmError, setConfirmError] = useState<UiText | null>(null);

  const chipRefs = useRef(new Map<string, HTMLButtonElement>());
  const pendingFocusRef = useRef<FocusAfterRemoval | null>(null);

  // confirm が変わるたびに、そちらのビューの最初のフォーカス先へ移す（開いた
  // 直後の最初のフォーカスは DialogContent の onOpenAutoFocus に任せるので、ここでは
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
  // きっかけは confirmError だけで、確認の表示が切り替わっても動かさない。
  const restoreFocus = useEffectEvent(() => {
    (confirm !== null ? confirmMergeButton : backRef).current?.focus();
  });
  useEffect(() => {
    if (confirmError !== null) restoreFocus();
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
      // 呼び出し元（TagsPage）の関数形の更新に任せる。ここで tagRef.current
      // から組み立てると、ほぼ同時に解除したもう1件の結果を踏みつぶす
      // （両方とも、送った時点の同じ古い一覧から組み立ててしまうため。N5）。
      onSynonymRemoved(tag.id, name);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      // 外せなかったときは、次/前のチップではなく、このチップ自身の × へ戻す
      // （`web/src/player/VideoTags.tsx` の removeTag と同じ扱い）。
      pendingFocusRef.current = { target: { name } };
      setAddError({ kind: "other", message: errorText(failure) });
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
   * `attempt` は、`submitAdd` からの自動の再送信（一覧を取り直したら名前の
   * 持ち主がもう無く、素の登録をやり直す）を1回までに区切る（N6b）。実際に
   * 送り直した回数だけを数える。この関数を呼ぶこと自体は数えないので、
   * `submitAdd` からの最初の呼び出しは常に `attempt = 0` で始まり、その中で
   * 見つからなければ1回だけ送り直せる。それでも見つからなければ、ループを
   * 続けず一般の失敗として見せる。
   */
  async function openConfirm(name: string, attempt = 0, ownerName?: string) {
    try {
      const list = await refreshTags();
      // S は API が tagName で返した元の名前で探す（contracts/error-api.md §1）。
      // tagName が無い応答では送った綴りで探す。
      const found = list.find((item) => item.name === (ownerName ?? name));
      if (found === undefined) {
        if (attempt >= 1) {
          setAddError({ kind: "other", message: t.tags.synonymsDialog.addFailed });
          return;
        }
        // 別のタブでの変更により、この名前を持つタグがもう無い。素の登録を
        // やり直せば通るはずなので、確認は出さずにもう一度試す（これが
        // 実際の送り直しなので、ここで attempt を1つ進める）。
        await submitAdd(name, attempt + 1);
        return;
      }
      setConfirm({
        name,
        sourceName: found.name,
        sourceId: found.id,
        source: found,
        videoCount: found.videoCount,
        hasSynonyms: found.synonyms.length > 0,
      });
    } catch (failure) {
      setAddError({ kind: "other", message: errorText(failure) });
    }
  }

  async function submitAdd(spelling?: string, attempt = 0) {
    const name = spelling ?? field.trySpelling();
    if (name === null) return;
    // 応答が届いたとき、まだこの値のままなら入力を空にする。届くまでの間に
    // 打ち直していれば、その文字を消さない。
    const submittedValue = fieldValueRef.current;
    setAddError(null);
    setAddPending(true);
    try {
      const updated = await addTagSynonym(tag.id, name);
      onTagUpdated(updated);
      if (fieldValueRef.current === submittedValue) field.setValue("");
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
        // openConfirm を呼ぶこと自体は送り直しではないので、attempt は
        // そのまま渡す（N6b: 実際に送り直した回数だけを数える）。
        await openConfirm(name, attempt, mergeOwner(failure));
        return;
      }
      setAddError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
    }
  }

  function backFromConfirm() {
    if (confirmPending) return;
    setConfirm(null);
    setConfirmError(null);
  }

  async function acceptMerge() {
    if (confirm === null) return;
    const submittedValue = fieldValueRef.current;
    setConfirmError(null);
    setConfirmPending(true);
    try {
      const updated = await addTagSynonym(tag.id, confirm.name, confirm.sourceId);
      // 統合元（confirm.sourceId）は統合先のシノニムになって一覧から消える。
      // 呼び出し元（TagsPage）へその id を渡し、一覧から取り除いてもらう
      // （渡さないと、統合元が背景の取り直しか、それが失敗すれば永久に
      // 一覧へ残ってしまう）。
      onTagUpdated(updated, confirm.source);
      setConfirm(null);
      if (fieldValueRef.current === submittedValue) field.setValue("");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      if (isMergeRequired(failure)) {
        // 確認の後に別のタブでこの名前がさらに別のタグへ移った。一覧を
        // 取り直して確認をやり直す（tags-api.md §3）。
        await openConfirm(confirm.name, 0, mergeOwner(failure));
        return;
      }
      setConfirmError(errorText(failure));
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
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        onEscapeKeyDown={(event) => {
          // IME の変換を取り消す Esc では閉じない。
          if (isComposingNativeKeyEvent(event)) event.preventDefault();
        }}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{t.tags.synonymsDialog.title(tag.name)}</DialogTitle>
        </DialogHeader>
        {confirm === null ? (
          <div className="flex min-w-0 flex-col gap-3">
            {tag.synonyms.length > 0 && (
              <ul
                aria-label={t.tags.synonymsDialog.list}
                className="flex flex-wrap gap-1"
              >
                {tag.synonyms.map((name) => (
                  <li key={name} className="max-w-full min-w-0">
                    {/* 外せるチップは Badge の中に × のボタンを置く（components.md「Badge」）。 */}
                    <Badge variant="secondary" title={name} className="max-w-full pr-0.5">
                      <span className="min-w-0 truncate">{name}</span>
                      <Button
                        ref={(node) => {
                          if (node) chipRefs.current.set(name, node);
                          else chipRefs.current.delete(name);
                        }}
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t.tags.synonymsDialog.remove(name)}
                        aria-busy={removing.has(name) || undefined}
                        onClick={() => {
                          if (removing.has(name)) return;
                          void removeSynonym(name);
                        }}
                        className="size-5 rounded-sm"
                      >
                        <X aria-hidden="true" />
                      </Button>
                    </Badge>
                  </li>
                ))}
              </ul>
            )}

            <Field data-invalid={field.reason !== null || addError !== null || undefined}>
              <FieldLabel htmlFor="synonym-add-input" className="sr-only">
                {t.tags.synonymsDialog.add}
              </FieldLabel>
              <div className="flex items-center gap-2">
                <Input
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
                  placeholder={t.tags.synonymsDialog.add}
                  aria-label={t.tags.synonymsDialog.add}
                  aria-describedby={
                    field.reason !== null
                      ? reasonId
                      : addError !== null
                        ? errorId
                        : undefined
                  }
                  aria-busy={addPending || undefined}
                  aria-invalid={
                    field.reason !== null || addError?.kind === "taken" || undefined
                  }
                  className="h-8"
                />
                <Button
                  size="sm"
                  onClick={() => {
                    if (!addPending) void submitAdd();
                  }}
                >
                  {t.tags.synonymsDialog.submit}
                </Button>
              </div>
              {field.reason !== null && (
                <FieldError id={reasonId} role={undefined}>
                  {field.reason}
                </FieldError>
              )}
              {field.reason === null &&
                addError !== null &&
                addError.kind === "taken" && (
                  <FieldError id={errorId} role={undefined}>
                    {addError.message}
                  </FieldError>
                )}
              {field.reason === null &&
                addError !== null &&
                addError.kind === "other" && (
                  <FieldError id={errorId} className="text-sm">
                    {addError.message}
                  </FieldError>
                )}
            </Field>
          </div>
        ) : (
          <>
            <p className="text-sm text-muted-foreground">
              {t.tags.synonymsDialog.mergeWarning(
                confirm.sourceName,
                confirm.videoCount,
                tag.name,
                confirm.hasSynonyms,
              )}
            </p>
            {confirmError !== null && <DialogError message={confirmError} />}
            <DialogFooter>
              <Button
                ref={backRef}
                variant="outline"
                size="sm"
                onClick={backFromConfirm}
                disabled={confirmPending}
              >
                {t.tags.synonymsDialog.back}
              </Button>
              <Button
                ref={confirmMergeButton}
                variant="destructive"
                size="sm"
                onClick={() => void acceptMerge()}
                disabled={confirmPending}
              >
                {confirmPending && <Spinner aria-hidden="true" />}
                {t.tags.synonymsDialog.merge}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
