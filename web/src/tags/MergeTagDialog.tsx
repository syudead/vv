import { ArrowRight, Search } from "lucide-react";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";

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
import { FormDialog } from "../ui/patterns/form-dialog";
import { Badge } from "../ui/shadcn/badge";
import { Field, FieldError, FieldLabel, FieldTitle } from "../ui/shadcn/field";
import { Spinner } from "../ui/shadcn/spinner";
import TagCommand, { type TagChoice } from "../ui/TagCommand";
import TentativeMark from "../ui/TentativeMark";
import { DialogError } from "./DialogError";

/**
 * MergeTagDialog は統合の確認の窓である。統合元（`sources`）を 1 件以上持ち、行の
 * 「別のタグへ統合…」（統合元 1 件）と選択バーの「Merge into one tag…」（統合元が
 * 選んだタグ）が同じ窓を開く（specs/036-tag-admin-scale/ui-design.md「Merge dialog」）。
 *
 * 統合先の候補は全部のタグから、入力のたびにサーバーの検索（`GET /api/tags?q=…&limit=8`）
 * で引く（画面が読み込んでいないタグも選べる。research.md R-14、ui-design.md「Target
 * candidates」）。並びはサーバーの名前の自然順のままで、入力と綴りが完全に一致するタグ
 * （応答の `exact`。部分一致の上位に入らなくても届く）だけを先頭に置く。
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
  // 見える名札が指す入力の id。cmdk が入力に自分の id を付けるので、描いた入力から読む。
  const targetLabelId = useId();
  const [targetInputId, setTargetInputId] = useState<string | undefined>(undefined);
  const targetInputRef = useCallback((node: HTMLInputElement | null) => {
    if (node !== null) setTargetInputId(node.id);
  }, []);
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Tag | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [impact, setImpact] = useState<TagImpactResponse | null>(null);
  const [countError, setCountError] = useState<UiText | null>(null);
  // 統合先の候補（最後に届いた検索の応答）。次の応答が届くまで前の候補を残す。
  const [candidates, setCandidates] = useState<readonly Tag[]>([]);
  // 最後に届いた応答の `exact`（入力と綴りが完全に一致するタグ）。
  const [exactCandidate, setExactCandidate] = useState<Tag | null>(null);
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
  const {
    tags: targetTags,
    options,
    exactOption,
  } = buildTargetOptions(candidates, exactCandidate, excluded, value);
  // 行から開いたときは応答から統合元を除くので、その分を多く引いて候補を 8 件に保つ。
  const requestLimit = Math.min(targetCandidateLimit + excluded.size, tagPageMaxLimit);

  // 入力が変わるたびに統合先の候補をサーバーで引く（窓を開いた直後の空の入力も同じ経路）。
  // 進行中の要求は次の入力で打ち切り、最後の応答だけを候補にする。
  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listTagPage({ q: value.trim(), limit: requestLimit }, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setExactCandidate(page.exact ?? null);
        setSearching(false);
      })
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        setSearchError(errorText(failure));
        setSearching(false);
      });
    return () => controller.abort();
  }, [value, requestLimit]);

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

  function selectTarget(option: TagChoice) {
    const found = targetTags.find((item) => String(item.id) === option.id);
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
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      title={title}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.tags.mergeDialog.submitting : t.tags.mergeDialog.submit}
        </>
      }
      pending={pending}
      submitDisabled={!canSubmit}
      submitRef={mergeButton}
      cancelRef={cancel}
      initialFocus={cancel}
    >
      {sources.length > 1 && (
        <Field>
          <FieldTitle id={sourcesHeadingId}>{t.tags.mergeDialog.sources}</FieldTitle>
          {/*
            統合元が多いときは並びの中だけを縦にスクロールさせ、統合先の欄と確認の文言を
            窓の中に残す（ui-design.md「Merge dialog」）。
          */}
          <ul
            aria-labelledby={sourcesHeadingId}
            className="flex max-h-16 flex-wrap gap-1 overflow-y-auto"
          >
            {sources.map((item) => (
              <li key={item.id} className="max-w-full min-w-0">
                <Badge variant="secondary" title={item.name} className="max-w-full">
                  <span className="min-w-0 truncate">{item.name}</span>
                  {item.tentative && (
                    <>
                      <TentativeMark />
                      <span className="sr-only">{t.tags.tentative}</span>
                    </>
                  )}
                  {item.id === targetId && (
                    <span className="font-normal text-muted-foreground">
                      {t.tags.mergeDialog.kept}
                    </span>
                  )}
                </Badge>
              </li>
            ))}
          </ul>
        </Field>
      )}
      <Field data-invalid={searchError !== null || undefined}>
        <FieldLabel id={targetLabelId} htmlFor={targetInputId}>
          {t.tags.mergeDialog.target}
        </FieldLabel>
        {/*
          統合先の候補はサーバーの検索で引き、入力の下の本文の中に高さを固定して並べる
          （ui-design.md「Merge dialog」「Target candidates」）。名前の検証と完全一致の扱いは
          ui/TagCommand のまま使う。Esc は窓が閉じる。
        */}
        <TagCommand
          layout="inline"
          label={t.tags.mergeDialog.target}
          labelledBy={targetLabelId}
          placeholder={null}
          inputRef={targetInputRef}
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
          choices={options}
          exactChoice={exactOption}
          onSelect={selectTarget}
          frameClassName="w-full"
          busy={searching}
          describedBy={searchError !== null ? searchErrorId : undefined}
        />
        {searchError !== null && (
          <FieldError id={searchErrorId} role="status" aria-live="polite">
            {t.tags.mergeDialog.searchFailed(searchError)}
          </FieldError>
        )}
      </Field>
      {target !== null && effective.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t.tags.mergeDialog.onlyTarget(target.name)}
        </p>
      )}
      {target !== null && kept && effective.length > 0 && (
        <p className="text-sm text-muted-foreground">
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
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Spinner aria-hidden="true" />
            {t.tags.bulkDialog.counting}
          </p>
        )}
      {countError !== null && (
        <DialogError
          message={t.tags.bulkDialog.countFailed(countError)}
          onRetry={() => setAttempt((current) => current + 1)}
        />
      )}
      {message !== null && <p className="text-sm text-muted-foreground">{message}</p>}
      {target !== null && effective.length > 0 && (
        // 統合元 → 統合先（ui-design.md「Merge dialog」）。
        <p className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
          <span className="min-w-0 truncate">
            {effective.length === 1
              ? effective[0]!.name
              : t.tags.mergeDialog.summarySources(effective.length)}
          </span>
          <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
          <span className="sr-only"> → </span>
          <span className="min-w-0 truncate font-medium text-foreground">
            {target.name}
          </span>
        </p>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}

/** targetCandidateLimit は統合先の候補に出す最大の行数（ui-design.md「Target candidates」）。 */
const targetCandidateLimit = 8;

/** tagPageMaxLimit は `GET /api/tags` の `limit` の上限（contracts/screen-api.md §5）。 */
const tagPageMaxLimit = 200;

/**
 * buildTargetOptions はサーバーの検索の応答から統合先の候補を作る。入力と綴りが完全に
 * 一致するタグ（応答の `exact`、無ければ `items` の中の一致）を先頭に置き、残りは応答の
 * まま（サーバーの名前の自然順）で、合わせて `targetCandidateLimit` 件まで。`exact` は
 * 前の入力への応答のこともあるので、今の入力と綴りが一致するときだけ使う。名前ではなく
 * シノニムで当たった候補には、照合形（`foldForMatch`。サーバーと同じ）で当たったシノニムを
 * 補足に添える。
 */
function buildTargetOptions(
  items: readonly Tag[],
  exact: Tag | null,
  excluded: ReadonlySet<number>,
  input: string,
): { tags: Tag[]; options: TagChoice[]; exactOption: TagChoice | null } {
  const trimmed = input.trim();
  const query = foldForMatch(trimmed);
  const spelled = (tag: Tag) =>
    trimmed !== "" && (tag.name === trimmed || tag.synonyms.includes(trimmed));
  const allowed = items.filter((tag) => !excluded.has(tag.id));

  const exactTag =
    exact !== null && !excluded.has(exact.id) && spelled(exact)
      ? exact
      : allowed.find(spelled);
  const candidates = (
    exactTag === undefined
      ? allowed
      : [exactTag, ...allowed.filter((tag) => tag.id !== exactTag.id)]
  ).slice(0, targetCandidateLimit);

  const options: TagChoice[] = candidates.map((tag) => {
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

  const exactOption: TagChoice | null =
    exactTag === undefined
      ? null
      : {
          id: String(exactTag.id),
          label: exactTag.name,
          meta: t.tags.mergeDialog.videoCount(exactTag.videoCount),
        };

  return { tags: candidates, options, exactOption };
}
