import { ArrowRight, LoaderCircle, Search } from "lucide-react";
import { useEffect, useEffectEvent, useId, useMemo, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import {
  listTagPage,
  mergeTag,
  tagImpact,
  type Tag,
  type TagImpactResponse,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import Button from "../ui/Button";
import Chip from "../ui/Chip";
import Combobox, { type ComboboxOption } from "../ui/Combobox";
import { ModalFrame } from "../ui/ModalFrame";
import TentativeMark from "../ui/TentativeMark";

/**
 * MergeTagDialog は統合の確認の窓である。統合元（`sources`）を 1 件以上持ち、行の
 * 「別のタグへ統合…」（統合元 1 件）と選択バーの「Merge into one tag…」（統合元が
 * 選んだタグ）が同じ窓を開く（specs/036-tag-admin-scale/ui-design.md「Merge dialog」）。
 *
 * 統合先の候補は全部のタグから、入力のたびにサーバーの検索（`GET /api/tags?q=…&limit=8`）
 * で引く（画面が読み込んでいないタグも選べる。research.md R-14、ui-design.md「Target
 * candidates」）。並びはサーバーの名前の自然順のままで、画面では並べ直さない。
 *
 * 行から開いた（`fromSelection` でない）ときは 014 の形のまま: 統合先の候補は応答から
 * 統合元を除いたもので、確認の本数は統合元の `videoCount`。選択から開いたときは、候補に
 * 選んだタグも残し、選んだ中のタグを統合先に選ぶとそのタグは統合元から外れる。統合先を選ぶたびに、
 * 統合先を外した統合元について `POST /api/tags/impact`（`merge`）で数え直し、届くまで
 * 「Merge」を押せなくする（数の無い確認で実行させない）。
 */
export default function MergeTagDialog({
  sources,
  fromSelection = false,
  onClose,
  onMerged,
  onStale,
}: {
  /** 統合元。窓を開いた時点で固定する（1 件以上）。 */
  sources: readonly Tag[];
  /** 選択バーから開いた。統合先の候補に統合元も含める。 */
  fromSelection?: boolean;
  onClose: () => void;
  /**
   * 統合が成功したときに呼ぶ。統合先の最新の状態、送った統合元の id、もう無かった
   * 統合元の id（`notFoundIds`。送った全部ではない）と、統合の前の統合先（選んだ候補）を
   * 渡す。呼び手は統合の前後の統合先をそれぞれ今の条件に照らして件数を数え直す
   * （specs/036-tag-admin-scale/data-model.md §4「操作のあとの反映」）。
   */
  onMerged: (
    merged: Tag,
    sourceIds: readonly number[],
    notFoundIds: readonly number[],
    target: Tag,
  ) => void;
  /**
   * 統合先がもう無い（tag_not_found）か、統合元がすべてもう無かった（応答の
   * notFoundIds が送った全部）ときに呼ぶ。
   */
  onStale: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  const mergeButton = useRef<HTMLButtonElement>(null);
  const sourcesHeadingId = useId();
  const searchErrorId = useId();
  const targetFieldId = useId();
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Tag | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [impact, setImpact] = useState<TagImpactResponse | null>(null);
  const [countError, setCountError] = useState<UiText | null>(null);
  // 統合先の候補（最後に届いた検索の応答）。次の応答が届くまで前の候補を残す。
  const [candidates, setCandidates] = useState<readonly Tag[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);
  // 数え直しのきっかけ。「Retry」と、統合先を選ぶたび（同じタグを選び直したときも。
  // targetId と effective が変わらないので、これが無いと前の数が残る）に進める。
  const [attempt, setAttempt] = useState(0);
  // 選んだ直後（フォーカスをまだ動かしていない）かを持つ。統合先を選ぶと
  // 「統合する」へフォーカスを移すが、それは候補を選んだ直後の1回だけで、
  // 統合先を選び直した（別の候補、または綴りの完全一致）ときにも1回だけ
  // 動かす（B: フォーカスは常に動く意味のある要素へ。候補の一覧を開いたまま
  // 確認の文言に重ねない）。数えている間は「統合する」が押せないので、数が
  // 届いて押せるようになってから動かす。
  const justSelectedRef = useRef(false);

  const excluded = useMemo(
    () => (fromSelection ? new Set<number>() : new Set(sources.map((item) => item.id))),
    [fromSelection, sources],
  );
  const { options, exactOption } = buildTargetOptions(candidates, excluded, value);

  // 入力が変わるたびに統合先の候補をサーバーで引く（窓を開いた直後の空の入力も同じ経路）。
  // 進行中の要求は次の入力で打ち切り、最後の応答だけを候補にする。
  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listTagPage({ q: value.trim(), limit: targetCandidateLimit }, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setSearching(false);
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setSearchError(errorText(failure));
        setSearching(false);
      });
    return () => controller.abort();
  }, [value]);

  const targetId = target?.id ?? null;
  /** 統合先を外した統合元。実際に送る `sourceIds` になる。 */
  const effective = useMemo(
    () => sources.filter((item) => item.id !== targetId),
    [sources, targetId],
  );
  const kept = targetId !== null && effective.length < sources.length;

  // 選択から開いたときは、統合先を選ぶたびに統合元の影響を数え直す。
  useEffect(() => {
    setImpact(null);
    setCountError(null);
    if (!fromSelection || targetId === null || effective.length === 0) return;
    const controller = new AbortController();
    tagImpact(
      "merge",
      effective.map((item) => item.id),
      controller.signal,
    )
      .then((result) => {
        if (!controller.signal.aborted) setImpact(result);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setCountError(errorText(failure));
      });
    return () => controller.abort();
  }, [fromSelection, targetId, effective, attempt]);

  const counted = !fromSelection || impact !== null;
  const canSubmit = target !== null && effective.length > 0 && counted && !pending;

  useEffect(() => {
    if (justSelectedRef.current && target !== null && canSubmit) {
      justSelectedRef.current = false;
      mergeButton.current?.focus();
    }
  }, [target, canSubmit]);

  // 統合が失敗した直後は、まだ有効な操作（統合先を選んだままなら「統合する」、
  // そうでなければ「キャンセル」）へフォーカスを戻す。何もしないと、失敗の
  // 直前に disabled にした「統合する」がフォーカスを失い、body に落ちる
  // （N4）。
  // きっかけは error だけで、統合先を選び直しても（target が変わっても）動かさない。
  const restoreFocus = useEffectEvent(() => {
    (target !== null ? mergeButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (error !== null) restoreFocus();
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
    const found = candidates.find((item) => String(item.id) === option.id);
    if (found === undefined) return;
    justSelectedRef.current = true;
    setTarget(found);
    // 統合先を選び直すたびに数え直す（ui-design.md「Confirmation」）。
    setAttempt((current) => current + 1);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || !canSubmit) return;
    const sourceIds = effective.map((item) => item.id);
    setError(null);
    setPending(true);
    try {
      const { tag: merged, notFoundIds } = await mergeTag(target.id, sourceIds);
      // 統合元がすべてもう無かったときは何も統合されていない。tag_not_found と同じく
      // 窓を閉じて一覧を取り直す（specs/036-tag-admin-scale/contracts/screen-api.md §2）。
      if (notFoundIds.length >= sourceIds.length) {
        onStale();
        return;
      }
      onMerged(merged, sourceIds, notFoundIds, target);
    } catch (failure) {
      if (failure instanceof RequestFailed && failure.code === "tag_not_found") {
        onStale();
        return;
      }
      setError(errorText(failure));
      setPending(false);
    }
  }

  const single = sources.length === 1 ? sources[0] : undefined;
  const title =
    single !== undefined
      ? t.tags.mergeDialog.title(single.name)
      : t.tags.mergeDialog.titleMany(sources.length);

  let message: UiText | null = null;
  if (target !== null && effective.length > 0 && counted) {
    const only = effective.length === 1 ? effective[0] : undefined;
    const videoCount = impact?.videoCount ?? only?.videoCount ?? 0;
    message =
      only !== undefined
        ? t.tags.mergeDialog.warning(only.name, videoCount, target.name)
        : t.tags.mergeDialog.warningMany(effective.length, videoCount, target.name);
  }

  return (
    <ModalFrame
      title={title}
      onClose={handleClose}
      initialFocus={cancel}
      width="sm:max-w-lg"
    >
      {/*
        候補の一覧（Combobox）は overflow-y-auto の外に置く。中に置くと、
        窓の中身がまだ短い（確認の文言が出る前）うちは、この div 自身の
        高さも短く、候補の一覧（最大8行）が overflow-y-auto によって
        そこで切り取られてしまう（B3）。確認の文言・失敗の行だけを別の
        小さな overflow-y-auto に包み、長い文言でもそちらだけが縦に
        スクロールする。統合元の並びは自分の max-h-32 で縦にスクロールする。
      */}
      <div className="flex min-h-0 flex-1 flex-col gap-3 p-4 sm:p-5">
        {sources.length > 1 && (
          <div>
            <p
              id={sourcesHeadingId}
              className="mb-1 text-xs font-semibold text-fg-muted uppercase"
            >
              {t.tags.mergeDialog.sources}
            </p>
            <ul
              aria-labelledby={sourcesHeadingId}
              className="flex max-h-32 flex-wrap gap-1.5 overflow-y-auto"
            >
              {sources.map((item) => (
                <li key={item.id} className="min-w-0 max-w-full">
                  <Chip tone="onElevated" title={item.name} className="max-w-full">
                    <span className="min-w-0 truncate">{item.name}</span>
                    {item.tentative && (
                      <>
                        <TentativeMark />
                        <span className="sr-only">{t.tags.tentative}</span>
                      </>
                    )}
                    {item.id === targetId && (
                      <span className="font-normal text-fg-muted">
                        {t.tags.mergeDialog.kept}
                      </span>
                    )}
                  </Chip>
                </li>
              ))}
            </ul>
          </div>
        )}
        <div className="shrink-0">
          <label
            htmlFor={targetFieldId}
            className="mb-1.5 block text-sm font-medium text-fg"
          >
            {t.tags.mergeDialog.target}
          </label>
          <Combobox
            id={targetFieldId}
            inline
            icon={<Search aria-hidden="true" />}
            chosenId={target === null ? undefined : String(target.id)}
            emptyText={
              searching || searchError !== null
                ? undefined
                : t.tags.mergeDialog.noCandidates
            }
            value={value}
            onValueChange={(next) => {
              setValue(next);
              setTarget(null);
              setError(null);
            }}
            options={options}
            exactOption={exactOption}
            onSelect={selectTarget}
            aria-label={t.tags.mergeDialog.target}
            // 候補の一覧が閉じているときの Esc は、この窓を閉じる
            // （B1。一覧が開いていれば Combobox 自身が一覧だけを閉じ、
            // preventDefault するので ModalFrame の Esc には届かない）。
            onEscapeWhenClosed={handleClose}
            className="w-full"
            // 統合先の入力と候補の一覧は窓の内側の幅いっぱい（要件 13、ui-design.md「Width」）。
            // 候補の一覧は入力の下の本文の中に高さを固定して置き、窓のボタンへ重ねない
            // （ui-design.md「Merge dialog」）。
            frameClassName="w-full"
            busy={searching}
            describedBy={searchError !== null ? searchErrorId : undefined}
          />
          {searchError !== null && (
            <p id={searchErrorId} aria-live="polite" className="mt-1 text-xs text-danger">
              {t.tags.mergeDialog.searchFailed(searchError)}
            </p>
          )}
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto">
          {target !== null && effective.length === 0 && (
            <p className="text-sm text-fg-muted">
              {t.tags.mergeDialog.onlyTarget(target.name)}
            </p>
          )}
          {target !== null && kept && effective.length > 0 && (
            <p className="text-sm text-fg-muted">
              {t.tags.mergeDialog.keptNote(target.name, effective.length)}
            </p>
          )}
          {fromSelection &&
            target !== null &&
            effective.length > 0 &&
            impact === null &&
            countError === null && (
              <p
                aria-busy="true"
                className="flex items-center gap-2 text-sm text-fg-muted"
              >
                <LoaderCircle
                  aria-hidden="true"
                  className="size-4 animate-spin motion-reduce:animate-none"
                />
                {t.tags.bulkDialog.counting}
              </p>
            )}
          {countError !== null && (
            <div className="flex flex-wrap items-center gap-2">
              <p role="alert" className="text-sm text-danger">
                {t.tags.bulkDialog.countFailed(countError)}
              </p>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setAttempt((current) => current + 1)}
              >
                {t.common.retry}
              </Button>
            </div>
          )}
          {message !== null && (
            <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
              {message}
            </p>
          )}
          {error !== null && (
            <p role="alert" className="text-sm text-danger">
              {error}
            </p>
          )}
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t border-border p-4">
        {target !== null && effective.length > 0 && (
          // 統合元 → 統合先（ui-design.md「Merge dialog」）。
          <p className="mr-auto flex min-w-0 items-center gap-1.5 text-sm text-fg-muted">
            <span className="min-w-0 truncate">
              {effective.length === 1
                ? effective[0]!.name
                : t.tags.mergeDialog.summarySources(effective.length)}
            </span>
            <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
            <span className="sr-only"> → </span>
            <span className="min-w-0 truncate font-medium text-fg">{target.name}</span>
          </p>
        )}
        <Button ref={cancel} onClick={handleClose} disabled={pending}>
          {t.common.cancel}
        </Button>
        <Button
          ref={mergeButton}
          variant="primary"
          onClick={() => void submit()}
          disabled={!canSubmit}
        >
          {pending && <LoaderCircle className="animate-spin" />}
          {pending ? t.tags.mergeDialog.submitting : t.tags.mergeDialog.submit}
        </Button>
      </div>
    </ModalFrame>
  );
}

/** targetCandidateLimit は統合先の候補に引く最大の行数（ui-design.md「Target candidates」）。 */
const targetCandidateLimit = 8;

/**
 * buildTargetOptions はサーバーの検索の応答から統合先の候補を作る。並びは応答のまま
 * （サーバーの名前の自然順）。名前ではなくシノニムで当たった候補には、照合形
 * （`foldForMatch`。サーバーと同じ）で当たったシノニムを補足に添える。
 */
function buildTargetOptions(
  tags: readonly Tag[],
  excluded: ReadonlySet<number>,
  input: string,
): { options: ComboboxOption[]; exactOption: ComboboxOption | null } {
  const trimmed = input.trim();
  const query = foldForMatch(trimmed);
  const candidates = tags.filter((tag) => !excluded.has(tag.id));

  const exactTag = candidates.find(
    (tag) => tag.name === trimmed || tag.synonyms.includes(trimmed),
  );

  const options: ComboboxOption[] = candidates.map((tag) => {
    const nameMatch = query === "" || foldForMatch(tag.name).includes(query);
    const synonymHit = nameMatch
      ? undefined
      : tag.synonyms.find((synonym) => foldForMatch(synonym).includes(query));
    return {
      id: String(tag.id),
      label: tag.name,
      hint:
        synonymHit !== undefined ? t.tags.mergeDialog.synonymHint(synonymHit) : undefined,
      meta: t.tags.mergeDialog.videoCount(tag.videoCount),
    };
  });

  const exactOption: ComboboxOption | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.tags.mergeDialog.videoCount(exactTag.videoCount),
        };

  return { options, exactOption };
}
