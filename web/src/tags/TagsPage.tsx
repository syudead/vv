import { AlertCircle, CircleDashed, Plus, SearchX, Tags as TagsIcon } from "lucide-react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { RequestFailed } from "../api/client";
import { compareTagRefs } from "../api/tagOrder";
import {
  confirmTag,
  createTag,
  currentTags,
  deleteTag,
  forgetRejectedTagName,
  listRejectedTagNames,
  refreshTags,
  rejectTag,
  renameTag,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import Skeleton from "../ui/Skeleton";
import { useToast } from "../ui/Toast";
import Tooltip from "../ui/Tooltip";
import { EmptyState } from "../videoList/states";
import CreateTagRow from "./CreateTagRow";
import DeleteTagDialog from "./DeleteTagDialog";
import MergeTagDialog from "./MergeTagDialog";
import RejectedNames from "./RejectedNames";
import RejectTagDialog from "./RejectTagDialog";
import SynonymsDialog from "./SynonymsDialog";
import { tagFieldError, type TagFieldError } from "./tagNameField";
import TagRow, { type TagRowRefs } from "./TagRow";
import TagSearchBox from "./TagSearchBox";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isTagNotTentative(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_tentative";
}

type FocusTarget = "rename" | "synonyms" | "name" | "menu";

/**
 * TagsPage はサイドバーの「タグ」から開く管理画面である（ui-design.md「Tag
 * management page」）。一覧・検索・作成・改名・削除・統合・シノニムの登録と
 * 解除を持つ。
 *
 * タグの一覧は共有の保持（`web/src/api/tags.ts`）を使う。作成・改名・削除の直後は、サーバーが返した最新の1件を
 * 今の一覧へその場で重ねる（もう1回 `GET /api/tags` を送らない。`createTag`・
 * `renameTag`・`deleteTag` 自体が共有の保持をバックグラウンドで取り直すので、
 * 二重の取得にはならない）。タグがもう無いとき（`tag_not_found`）だけ、
 * ほかのタグも変わっているかもしれないので `reload` で取り直す。
 *
 * 仮のタグ（specs/031-tentative-tags/ui-design.md「Tag management page」）:
 * 「Tentative only」の絞り込み（この画面の状態で URL には載せない）、仮の行の
 * 確定・却下、一覧の下の却下した名前の一覧と取り外しも持つ。却下した名前の
 * 一覧はこの画面だけが読むので、共有の保持ではなくここで取る（research.md R-8）。
 */
export default function TagsPage() {
  const toast = useToast();
  const [tags, setTags] = useState<Tag[] | undefined>(currentTags());
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [search, setSearch] = useState("");
  const [tentativeOnly, setTentativeOnly] = useState(false);

  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);

  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);

  const [deletingTag, setDeletingTag] = useState<Tag | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  const [mergingTag, setMergingTag] = useState<Tag | null>(null);
  const [synonymsTagId, setSynonymsTagId] = useState<number | null>(null);
  /** シノニムの窓を開いたときの並び。閉じたときに行が外れていればここから次の行を探す。 */
  const synonymsOrderRef = useRef<readonly Tag[]>([]);

  const [confirming, setConfirming] = useState<ReadonlySet<number>>(new Set());
  const confirmingRef = useRef(new Set<number>());

  const [rejectingTag, setRejectingTag] = useState<Tag | null>(null);
  const [rejectPending, setRejectPending] = useState(false);
  const [rejectError, setRejectError] = useState<UiText | null>(null);

  const [rejectedNames, setRejectedNames] = useState<string[] | undefined>(undefined);
  const [rejectedError, setRejectedError] = useState<UiText | null>(null);
  const rejectedGeneration = useRef(0);
  /** 取り直しの応答を待っている世代。待っていなければ null。 */
  const rejectedInFlight = useRef<number | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  const tentativeButtonRef = useRef<HTMLButtonElement | null>(null);
  const rowRefs = useRef(new Map<number, TagRowRefs>());

  const registerRefs = useCallback((id: number, refs: Partial<TagRowRefs>) => {
    const current = rowRefs.current.get(id) ?? {
      nameLink: null,
      renameButton: null,
      synonymsButton: null,
      menuButton: null,
    };
    rowRefs.current.set(id, { ...current, ...refs });
  }, []);

  /**
   * filtersRef は今描いている検索と「Tentative only」である。要求の応答を
   * 待つ間に絞り込みが変わることがあるので、応答のあとのフォーカス先は
   * 閉じ込めた（押した時点の）値ではなくこれで決める。
   */
  const filtersRef = useRef({ query: "", tentativeOnly: false });

  /**
   * fallbackFocus は、行へ移せないときの最後の行き先である。ふだんは
   * 「新しいタグ」、「Tentative only」を押している間はそのボタン（次の操作が
   * 「絞り込みを外す」だから。specs/031-tentative-tags/ui-design.md「Toolbar」）。
   */
  function fallbackFocus() {
    (filtersRef.current.tentativeOnly
      ? tentativeButtonRef
      : createButtonRef
    ).current?.focus();
  }

  /**
   * focusRow はタグの行のフォーカス先へ移す。その行がもう無ければ
   * `fallbackFocus` へ移す。行の差し替えが DOM に反映されたあとで移す必要が
   * あるので setTimeout(0) で1呼吸置く（settings/SettingsPage.tsx の
   * focusFolderAction と同じ）。
   */
  function focusRow(id: number, part: FocusTarget) {
    setTimeout(() => {
      const refs = rowRefs.current.get(id);
      const target =
        part === "rename"
          ? refs?.renameButton
          : part === "synonyms"
            ? refs?.synonymsButton
            : part === "menu"
              ? refs?.menuButton
              : refs?.nameLink;
      if (target === null || target === undefined) fallbackFocus();
      else target.focus();
    }, 0);
  }

  /**
   * focusAfterRemoval は、行が一覧から消えた（削除・却下、tag_not_found の
   * 取り直しで消えた、または「Tentative only」の絞り込みから外れた）あとの
   * フォーカス先を決める。次の行の「改名」、無ければ前の行、1つも無ければ
   * `fallbackFocus`（ui-design.md「Merge and delete」、031 の「Toolbar」）。
   * order は消える前の（絞り込み後の）並びで、`alsoGone` は同時に一覧から
   * 外れるほかの行（統合先が確定になって絞り込みから外れるときなど）である。
   *
   * 候補は、移す時点でまだ描かれていて押せる「改名」に限る。ほかの行の確定が
   * 並行していると、order を控えたあとにその行も外れていたり、送信中で
   * 「改名」が disabled だったりするので、それを飛ばして次の行へ進む。
   */
  function focusAfterRemoval(
    order: readonly Tag[],
    removedId: number,
    alsoGone: ReadonlySet<number> = new Set(),
  ) {
    const index = order.findIndex((tag) => tag.id === removedId);
    const stays = (tag: Tag) => tag.id !== removedId && !alsoGone.has(tag.id);
    const candidates = [
      ...order.slice(index + 1).filter(stays),
      ...order.slice(0, Math.max(index, 0)).reverse().filter(stays),
    ];
    setTimeout(() => {
      const target = candidates
        .map((tag) => rowRefs.current.get(tag.id)?.renameButton)
        .find((button) => button?.isConnected === true && !button.disabled);
      if (target === null || target === undefined) fallbackFocus();
      else target.focus();
    }, 0);
  }

  /**
   * reloadRejectedNames は却下した名前の一覧を取り直す。却下・作成・改名・
   * シノニムの追加のあと（どれも一覧を変えうる。要件 15、受け入れ条件 14）と、
   * 画面を開いたときに呼ぶ。追い越された古い取得の結果は捨てる。取り直しの
   * 失敗は、最初の読み込みの失敗と同じ見え方（件数を出さず、「Couldn't load
   * the rejected names」と Retry。ui-design.md「Rejected names」）にする。
   * 古い一覧を黙って残すと、却下や作成で変わったはずの並びを正しいものとして
   * 見せ続けてしまう。
   */
  const reloadRejectedNames = useCallback(() => {
    rejectedGeneration.current += 1;
    const generation = rejectedGeneration.current;
    rejectedInFlight.current = generation;
    setRejectedError(null);
    listRejectedTagNames()
      .then((names) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        setRejectedNames(names);
      })
      .catch((failure: unknown) => {
        if (generation !== rejectedGeneration.current) return;
        rejectedInFlight.current = null;
        setRejectedNames(undefined);
        setRejectedError(errorText(failure));
      });
  }, []);

  /**
   * forgetRejectedName は × の取り外しである。取り外しの前から待っている
   * 取り直しがあれば、その応答は取り外す前の並び（外した名前を含む）かも
   * しれないので捨て、取り外しのあとで取り直す。
   */
  async function forgetRejectedName(name: string) {
    await forgetRejectedTagName(name);
    setRejectedNames((current) => current?.filter((item) => item !== name));
    if (rejectedInFlight.current !== null) reloadRejectedNames();
  }

  const reload = useCallback(() => {
    setLoadError(null);
    // 一覧への反映は下の mount の subscribeTags に一本化し、ここでは
    // `refreshTags` の戻り値を直接 `setTags` へは使わない。この呼び出しが
    // 別の（後から始まった）取り直しに追い越されると、`refreshTags` の
    // generation ガードはこの呼び出し自身の取得結果ではなく、その時点の
    // `held`（追い越した側がまだ終わっていなければ、さらに古い値）を返す。
    // 直前に作成したタグをその場で重ねた直後にこれが起きると、その重ねを
    // 古い一覧で上書きしてしまう（Devin の指摘4）。`subscribeTags` の通知は
    // 常に「実際に held を更新した、最新の取得」でしか呼ばれないので、
    // そちらだけを信頼する。
    return refreshTags()
      .then(() => undefined)
      .catch((failure: unknown) => {
        // 既に一覧を持っているときは、その一覧を残したまま理由だけを控える
        // （読み込み失敗の空の状態は、一覧をまだ一度も取れていないときだけ
        // 出す。N6: 直前の操作は成功しているので、一覧を空白にしない）。
        setLoadError(errorText(failure));
        return undefined;
      });
  }, []);

  useEffect(() => {
    reloadRejectedNames();
  }, [reloadRejectedNames]);

  useEffect(() => {
    let alive = true;
    void reload();
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, [reload]);

  // 一覧は `GET /api/tags` が返す名前の自然順（contracts/tags-api.md §3）を
  // 保つが、作成・改名でその場に重ねた1件は並びの外にあるかもしれないので
  // `compareTagRefs`（カード・再生画面・候補と同じ並び替え）で並べ直す。
  const sorted = useMemo(() => {
    if (tags === undefined) return [];
    return [...tags].sort(compareTagRefs);
  }, [tags]);

  const normalizedQuery = search.trim().toLowerCase();
  const filtered = useMemo(
    () => sorted.filter((tag) => matchesFilters(tag, normalizedQuery, tentativeOnly)),
    [sorted, normalizedQuery, tentativeOnly],
  );

  useLayoutEffect(() => {
    filtersRef.current = { query: normalizedQuery, tentativeOnly };
  }, [normalizedQuery, tentativeOnly]);

  /**
   * shown は、その1件が今の検索と「Tentative only」の絞り込みで一覧に出るかで
   * ある。要求の応答のあとで呼ぶので、`filtersRef` の今の値で決める。
   */
  function shown(tag: Tag): boolean {
    const { query, tentativeOnly: onlyTentative } = filtersRef.current;
    return matchesFilters(tag, query, onlyTentative);
  }

  /**
   * visibleRows は実際に並べる行である。改名中の行は、検索を変えて一致しなく
   * なっても一覧から外さない（外すとその行が消え、打っている途中の名前を
   * 失う）。`filtered` に無ければ `sorted` から拾い、並び（`compareTagRefs`）を
   * 保つ位置へ差し込む。件数の行はこれを数えず、実際の一致件数（`filtered`）
   * のまま見せる。
   */
  const visibleRows = useMemo(() => {
    if (renamingId === null) return filtered;
    if (filtered.some((tag) => tag.id === renamingId)) return filtered;
    const renamingTag = sorted.find((tag) => tag.id === renamingId);
    if (renamingTag === undefined) return filtered;
    const insertAt = filtered.findIndex((tag) => compareTagRefs(tag, renamingTag) > 0);
    const at = insertAt === -1 ? filtered.length : insertAt;
    return [...filtered.slice(0, at), renamingTag, ...filtered.slice(at)];
  }, [filtered, sorted, renamingId]);

  const searching = normalizedQuery !== "";
  const total = tags?.length ?? 0;
  const countText =
    tags === undefined
      ? t.tags.loading
      : searching || tentativeOnly
        ? t.tags.filteredCount(filtered.length, total)
        : t.tags.count(total);

  function openCreate() {
    // 改名の送信中は、その応答が届くまで新しく作成を始めない（B2 と同じ規則。
    // 「新しいタグ」自体も createPending・creating では disabled だが、
    // renamePending はボタンの disabled 条件に含めているので、ここは主に
    // キーボード操作などボタンを介さない呼び出しへの保険である）。
    if (renamePending) return;
    setCreating(true);
    setCreateError(null);
    setRenamingId(null);
  }

  function clearSearch() {
    setSearch("");
    searchInputRef.current?.focus();
  }

  /**
   * toggleTentativeOnly は「Tentative only」を押す・外す。外した結果タグが
   * 1つも無ければボタンは disabled になるので、フォーカスを「新しいタグ」へ
   * 移す（031 の ui-design.md「Toolbar」）。
   */
  function toggleTentativeOnly() {
    if (tentativeOnly && total === 0) {
      setTimeout(() => createButtonRef.current?.focus(), 0);
    }
    setTentativeOnly((value) => !value);
  }

  /**
   * showAllFromTentative は「No tentative tags」の「Show all tags」である。
   * 絞り込みを外し、「Tentative only」へ（タグが 0 なら「新しいタグ」へ）移す。
   */
  function showAllFromTentative() {
    setTentativeOnly(false);
    setTimeout(
      () => (total === 0 ? createButtonRef : tentativeButtonRef).current?.focus(),
      0,
    );
  }

  /**
   * showAllFromTentativeSearch は「No tentative tags match」の「Show all tags」
   * である。絞り込みと検索の両方を外し、検索の入力へ移す。
   */
  function showAllFromTentativeSearch() {
    setTentativeOnly(false);
    setSearch("");
    setTimeout(
      () => (total === 0 ? createButtonRef : searchInputRef).current?.focus(),
      0,
    );
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTag(name);
      setTags((current) => (current === undefined ? [created] : [...current, created]));
      setCreating(false);
      reloadRejectedNames();
      focusRow(created.id, "name");
    } catch (failure) {
      setCreateError(tagFieldError(failure, { submitted: name }));
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(tag: Tag, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = visibleRows;
    try {
      const updated = await renameTag(tag.id, name);
      setTags((current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
      setRenamingId(null);
      reloadRejectedNames();
      // 改名で確定になった仮のタグは「Tentative only」から外れる（031 の
      // ui-design.md「Toolbar」）。検索に一致しなくなったときも同じ扱い。
      if (shown(updated)) focusRow(tag.id, "rename");
      else focusAfterRemoval(order, tag.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRenamingId(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      setRenameError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deletingTag === null) return;
    const target = deletingTag;
    const order = visibleRows;
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTag(target.id);
      setTags((current) => current?.filter((item) => item.id !== target.id));
      setDeletingTag(null);
      toast(t.tags.deleted(target.name));
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setDeletingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      setDeleteError(errorText(failure));
    } finally {
      setDeletePending(false);
    }
  }

  /**
   * submitConfirm は仮の行の「確定する」を扱う（031 の ui-design.md「Confirm」）。
   * 送信中の行は二度押しを無視する（`confirmingRef` は同じ呼吸の連打も
   * 止める）。成功したら行を差し替え、同じ行の「改名」へ、絞り込みから
   * 外れたら次の行の「改名」へフォーカスを移す。
   */
  async function submitConfirm(tag: Tag) {
    if (confirmingRef.current.has(tag.id)) return;
    confirmingRef.current.add(tag.id);
    setConfirming(new Set(confirmingRef.current));
    const order = visibleRows;
    try {
      const updated = await confirmTag(tag.id);
      setTags((current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
      toast(t.tags.confirmed(updated.name));
      if (shown(updated)) focusRow(updated.id, "rename");
      else focusAfterRemoval(order, updated.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      // 行の中に失敗の1行を置く場所が無いので、トーストで伝える。
      toast(errorText(failure));
    } finally {
      confirmingRef.current.delete(tag.id);
      setConfirming(new Set(confirmingRef.current));
    }
  }

  /** performReject は却下の窓の「Reject」である（031 の ui-design.md「Reject」）。 */
  async function performReject() {
    if (rejectingTag === null) return;
    const target = rejectingTag;
    const order = visibleRows;
    setRejectError(null);
    setRejectPending(true);
    try {
      await rejectTag(target.id);
      setTags((current) => current?.filter((item) => item.id !== target.id));
      setRejectingTag(null);
      toast(t.tags.rejected(target.name));
      reloadRejectedNames();
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRejectingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      if (isTagNotTentative(failure)) {
        // 別のタブや API で先に確定された（Edge Case「操作の競合」）。
        // 行は確定したタグとして残り、「Tentative only」中は一覧から外れる。
        setRejectingTag(null);
        toast(t.tags.alreadyConfirmed);
        await reload();
        if (filtersRef.current.tentativeOnly) focusAfterRemoval(order, target.id);
        else focusRow(target.id, "menu");
        return;
      }
      setRejectError(errorText(failure));
    } finally {
      setRejectPending(false);
    }
  }

  /** cancelReject は却下の窓を何も変えずに閉じ、その行の「その他の操作」へ戻す。 */
  function cancelReject() {
    if (rejectPending || rejectingTag === null) return;
    const target = rejectingTag;
    setRejectingTag(null);
    setRejectError(null);
    focusRow(target.id, "menu");
  }

  function cancelDelete() {
    if (deletePending || deletingTag === null) return;
    const target = deletingTag;
    setDeletingTag(null);
    setDeleteError(null);
    // 窓は「その他の操作」のメニューの項目から開いた。その項目はメニューが
    // 閉じるともう無いので、ModalFrame の「前のフォーカスへ戻す」には頼らず、
    // その行の「その他の操作」へ明示的に戻す（B2）。
    focusRow(target.id, "menu");
  }

  /**
   * cancelMerge は統合の確認の窓を、何も変えずに閉じる（キャンセル・Esc）。
   * 「別のタグへ統合…」は「その他の操作」のメニューの項目から開いたので、
   * cancelDelete と同じくその行の「その他の操作」へ明示的に戻す（B2）。
   */
  function cancelMerge() {
    if (mergingTag === null) return;
    const target = mergingTag;
    setMergingTag(null);
    focusRow(target.id, "menu");
  }

  /**
   * performMerge は統合が成功したときに呼ぶ。統合元は一覧から消え、統合先は
   * サーバーが返した最新の状態（シノニムに統合元の名前を含む）に差し替わる
   * （ui-design.md「Merge and delete」、受け入れ条件 12）。
   */
  function performMerge(merged: Tag) {
    const source = mergingTag;
    if (source === null) return;
    const order = visibleRows;
    setTags((current) =>
      current
        ?.filter((item) => item.id !== source.id)
        .map((item) => (item.id === merged.id ? merged : item)),
    );
    setMergingTag(null);
    toast(t.tags.merged(source.name, merged.name));
    // 統合先は確定になるので、「Tentative only」を押している間は統合元・
    // 統合先のどちらも一覧に無い。そのときは統合元の位置から次の行へ
    // （031 の ui-design.md「Merge」）。
    if (shown(merged)) focusRow(merged.id, "name");
    else focusAfterRemoval(order, source.id, new Set([merged.id]));
  }

  /**
   * staleMerge は統合元・統合先のどちらかがもう無かった（tag_not_found）ときに
   * 呼ぶ。ほかの操作の tag_not_found と同じく、窓を閉じてトーストを出し、
   * 一覧を取り直す（ui-design.md「States」）。
   */
  function staleMerge() {
    const source = mergingTag;
    if (source === null) return;
    const order = filtered;
    setMergingTag(null);
    toast(t.tags.gone);
    void reload().then(() => focusAfterRemoval(order, source.id));
  }

  /**
   * cancelSynonyms はシノニムの窓を閉じる。「シノニム」はその行に直接置いた
   * ボタンなので（メニューの項目ではない）、その行の「シノニム」へ明示的に
   * 戻す。シノニムの追加で確定になり「Tentative only」から外れていれば、
   * 窓を開いたときの並びから次の行の「改名」へ移す（031 の ui-design.md
   * 「Toolbar」。窓を開いている間はフォーカスを窓の中に残す）。
   */
  function cancelSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    setSynonymsTagId(null);
    if (filtered.some((tag) => tag.id === id)) {
      focusRow(id, "synonyms");
      return;
    }
    const visibleIds = new Set(filtered.map((tag) => tag.id));
    const order = synonymsOrderRef.current;
    const gone = new Set(
      order.filter((tag) => !visibleIds.has(tag.id)).map((tag) => tag.id),
    );
    focusAfterRemoval(order, id, gone);
  }

  /**
   * updateSynonymsTag はシノニムの登録・解除・シノニム登録に伴う統合が
   * 成功したときに、一覧の中のその1件を差し替える（`web/src/api/tags.ts` の
   * 各関数がバックグラウンドで共有の一覧も取り直すが、ここではその結果を
   * 待たずに画面へその場で反映する。作成・改名・削除と同じ扱い）。
   *
   * `removedId` は、シノニム登録に伴う統合（承諾したとき）でだけ渡す。統合元
   * のタグは統合先のシノニムになって一覧から消えるので、そのタグを一覧から
   * 取り除いてから統合先を差し替える。渡さなければ（素のシノニムの登録・
   * 解除）何も取り除かない。取り除かないと、統合元がバックグラウンドの
   * 取り直し（または、それが失敗すれば永久）まで一覧に残ってしまう。
   */
  function updateSynonymsTag(updated: Tag, removedId?: number) {
    setTags((current) => {
      if (current === undefined) return current;
      const withoutRemoved =
        removedId === undefined
          ? current
          : current.filter((item) => item.id !== removedId);
      return withoutRemoved.map((item) => (item.id === updated.id ? updated : item));
    });
    // シノニムに足した名前が却下した名前だったなら、一覧から外れる（要件 15）。
    reloadRejectedNames();
  }

  /**
   * removeSynonymFromTag は、1件のシノニムの解除が成功したときに呼ぶ。
   * `SynonymsDialog` に渡した `tag` の閉じ込め（古いかもしれない）ではなく、
   * `setTags` の関数形で常に最新の一覧からその名前だけを取り除く。複数の
   * シノニムをほぼ同時に解除したとき、それぞれの応答が別々にここへ届いても、
   * 互いの結果を巻き戻さない（N5・並行する解除）。
   */
  function removeSynonymFromTag(tagId: number, name: string) {
    setTags((current) =>
      current?.map((item) =>
        item.id === tagId
          ? { ...item, synonyms: item.synonyms.filter((s) => s !== name) }
          : item,
      ),
    );
  }

  /**
   * staleSynonyms は、シノニムの窓を開いていたタグがもう無かった
   * （tag_not_found）ときに呼ぶ。
   */
  function staleSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    const order = filtered;
    setSynonymsTagId(null);
    toast(t.tags.gone);
    void reload().then(() => focusAfterRemoval(order, id));
  }

  const showEmptyTags =
    tags !== undefined && tags.length === 0 && !creating && !tentativeOnly;
  const nothingShown =
    tags !== undefined && visibleRows.length === 0 && !creating && !showEmptyTags;
  const showNoTentative = nothingShown && tentativeOnly && !searching;
  const showNoTentativeMatch = nothingShown && tentativeOnly && searching;
  const showNoMatch = nothingShown && !tentativeOnly;
  const showRows =
    tags !== undefined && (visibleRows.length > 0 || creating) && !showEmptyTags;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-6 sm:py-8">
      <h1 className="text-xl font-semibold">{t.tags.title}</h1>
      {/*
        h1・操作の行・件数の行の間隔は ui-design.md が固定していない（固定するのは
        本文の外側の余白と行の py-2 だけ）。「1280×800 でシノニムの行を持つタグと
        持たないタグが半々のとき、12 行以上が1画面に見える」（ui-design.md「Visual
        review criteria」情報密度）を満たすため、ここを詰める（B5）。
      */}

      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
        <TagSearchBox
          value={search}
          onChange={setSearch}
          inputRef={searchInputRef}
          disabled={tags !== undefined && tags.length === 0}
          className="w-full min-w-0 sm:max-w-sm sm:flex-1"
        />
        {/*
          sm 未満は「Tentative only」と「新しいタグ」を2行目に半分ずつ、sm 以上は
          検索の右に「Tentative only」、右端に「新しいタグ」（031 の ui-design.md
          「Toolbar」）。
        */}
        <div className="grid grid-cols-2 gap-2 sm:flex sm:flex-1 sm:items-center sm:justify-between sm:gap-3">
          <Tooltip content={t.tags.tentativeOnlyHint}>
            <Button
              ref={tentativeButtonRef}
              aria-pressed={tentativeOnly}
              className="w-full aria-pressed:border-accent-active aria-pressed:bg-accent-soft aria-pressed:text-link sm:w-auto"
              onClick={toggleTentativeOnly}
              // 押している間は、タグが 0 になっても disabled にしない
              // （フォーカスの行き先で、絞り込みを外す唯一の手でもある）。
              disabled={tags === undefined || (!tentativeOnly && tags.length === 0)}
            >
              <CircleDashed />
              {t.tags.tentativeOnly}
            </Button>
          </Tooltip>
          <Button
            ref={createButtonRef}
            variant="primary"
            className="w-full sm:w-auto"
            onClick={openCreate}
            disabled={tags === undefined || creating || createPending || renamePending}
          >
            <Plus />
            {t.tags.newTag}
          </Button>
        </div>
      </div>

      <p
        role="status"
        aria-live="polite"
        className="mt-2 text-xs text-fg-muted tabular-nums"
      >
        {countText}
      </p>

      <div className="mt-2">
        {tags === undefined && loadError === null && (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 6 }, (_, index) => (
              <Skeleton key={index} className="h-10" />
            ))}
          </div>
        )}

        {tags === undefined && loadError !== null && (
          <EmptyState
            icon={AlertCircle}
            tone="danger"
            title={t.tags.loadFailed}
            action={<Button onClick={() => void reload()}>{t.common.retry}</Button>}
          />
        )}

        {showEmptyTags && (
          <EmptyState
            icon={TagsIcon}
            title={t.tags.empty.title}
            description={t.tags.empty.description}
            action={
              <Button variant="primary" onClick={openCreate}>
                <Plus />
                {t.tags.newTag}
              </Button>
            }
          />
        )}

        {showNoTentative && (
          <EmptyState
            icon={CircleDashed}
            title={t.tags.noTentative.title}
            description={t.tags.noTentative.description}
            action={<Button onClick={showAllFromTentative}>{t.tags.clearSearch}</Button>}
          />
        )}

        {showNoTentativeMatch && (
          <EmptyState
            icon={SearchX}
            title={t.tags.noTentativeMatches(search)}
            action={
              <Button onClick={showAllFromTentativeSearch}>{t.tags.clearSearch}</Button>
            }
          />
        )}

        {showNoMatch && (
          <EmptyState
            icon={SearchX}
            title={t.tags.noMatches(search)}
            action={<Button onClick={clearSearch}>{t.tags.clearSearch}</Button>}
          />
        )}

        {showRows && (
          <div className="divide-y divide-border">
            {creating && (
              <CreateTagRow
                pending={createPending}
                error={createError}
                onCancel={() => {
                  // 送信中は、その応答が届くまで閉じない（B2 と同じ規則。
                  // CreateTagRow 自身の Esc・「キャンセル」も pending を見るが、
                  // ここでも二重に守る）。
                  if (createPending) return;
                  setCreating(false);
                  setCreateError(null);
                  // 作成を始めた「新しいタグ」へ戻す（「Tentative only」を
                  // 押していても。そちらは行が外れたときの行き先である）。
                  // creating が false になって押せるようになってから移す。
                  setTimeout(() => createButtonRef.current?.focus(), 0);
                }}
                onSubmit={(name) => void submitCreate(name)}
                onDraftChange={() => setCreateError(null)}
              />
            )}
            {visibleRows.map((tag) => (
              <TagRow
                key={tag.id}
                tag={tag}
                renaming={renamingId === tag.id}
                pending={renamingId === tag.id && renamePending}
                blockStart={createPending || (renamePending && renamingId !== tag.id)}
                error={renamingId === tag.id ? renameError : null}
                registerRefs={registerRefs}
                onStartRename={(target) => {
                  // ほかの行の改名や作成が送信中は、新しく改名を始めない
                  // （B2 と同じ規則）。行の「改名」自体も blockStart で
                  // disabled だが、ここでも二重に守る。
                  if (createPending || renamePending) return;
                  setCreating(false);
                  setRenameError(null);
                  setRenamingId(target.id);
                }}
                onCancelRename={() => {
                  if (renamePending) return;
                  setRenamingId(null);
                  setRenameError(null);
                  focusRow(tag.id, "rename");
                }}
                onSubmitRename={(target, name) => void submitRename(target, name)}
                onOpenSynonyms={(target) => {
                  synonymsOrderRef.current = visibleRows;
                  setSynonymsTagId(target.id);
                }}
                onOpenMerge={(target) => setMergingTag(target)}
                onDelete={(target) => {
                  setDeleteError(null);
                  setDeletingTag(target);
                }}
                confirming={confirming.has(tag.id)}
                onConfirm={(target) => void submitConfirm(target)}
                onReject={(target) => {
                  setRejectError(null);
                  setRejectingTag(target);
                }}
                onDraftChange={() => setRenameError(null)}
              />
            ))}
          </div>
        )}
      </div>

      {/*
        却下した名前は、タグの一覧をまだ一度も取れていない間（読み込み中・
        読み込み失敗）は置かない。「タグはまだありません」のときは置く
        （031 の ui-design.md「Rejected names」）。
      */}
      {tags !== undefined && (
        <RejectedNames
          names={rejectedNames}
          error={rejectedError}
          onRetry={reloadRejectedNames}
          onForget={forgetRejectedName}
        />
      )}

      {deletingTag !== null && (
        <DeleteTagDialog
          tag={deletingTag}
          pending={deletePending}
          error={deleteError}
          onClose={cancelDelete}
          onDelete={() => void performDelete()}
        />
      )}

      {rejectingTag !== null && (
        <RejectTagDialog
          tag={rejectingTag}
          pending={rejectPending}
          error={rejectError}
          onClose={cancelReject}
          onReject={() => void performReject()}
        />
      )}

      {mergingTag !== null && tags !== undefined && (
        <MergeTagDialog
          source={mergingTag}
          tags={tags}
          onClose={cancelMerge}
          onMerged={performMerge}
          onStale={staleMerge}
        />
      )}

      {synonymsTagId !== null &&
        (() => {
          const synonymsTag = tags?.find((item) => item.id === synonymsTagId);
          // 別のタブでの削除・統合と、staleSynonyms による一覧の取り直しの
          // 間に、そのタグがもう一覧に無い一瞬がありうる。窓はまだ閉じ切って
          // いないその一瞬だけ何も出さない。
          if (synonymsTag === undefined) return null;
          return (
            <SynonymsDialog
              tag={synonymsTag}
              onClose={cancelSynonyms}
              onTagUpdated={updateSynonymsTag}
              onSynonymRemoved={removeSynonymFromTag}
              onStale={staleSynonyms}
            />
          );
        })()}
    </div>
  );
}

/**
 * matchesFilters は、タグが検索（名前かシノニムの部分一致、大文字小文字を
 * 区別しない）と「Tentative only」の両方に一致するかである。
 */
function matchesFilters(
  tag: Tag,
  normalizedQuery: string,
  tentativeOnly: boolean,
): boolean {
  if (tentativeOnly && !tag.tentative) return false;
  if (normalizedQuery === "") return true;
  return (
    tag.name.toLowerCase().includes(normalizedQuery) ||
    tag.synonyms.some((synonym) => synonym.toLowerCase().includes(normalizedQuery))
  );
}
