import type {
  FolderListing,
  Progress,
  TagRef,
  Video,
  VideoSort,
  WatchFilter,
} from "./client";
import { applyTagToTags } from "./tagOrder";

/**
 * defaultSort は並び順が指定されていないときの値である（data-model.md 1.）。
 *
 * 鍵の正規化がこの既定を吸収する。`/` と `/?sort=addedDesc` は利用者から見て
 * 同じ一覧なので、片方で保存した控えをもう片方で拾えないと復元が取りこぼす。
 */
const defaultSort: VideoSort = "addedDesc";

/**
 * ListKey は一覧を一意に決める条件である（検索語・視聴状態・再生可否・並び順・
 * seed、フォルダ画面ではフォルダも。specs/013-library-search/contracts/list-url.md）。
 */
export interface ListKey {
  /** 検索語。前後の空白は鍵の正規化が落とす。 */
  query: string;
  /** 視聴状態。省略すると all と同じ鍵になる。 */
  watch?: WatchFilter;
  /** 再生できるものだけか。省略すると偽と同じ鍵になる。 */
  playable?: boolean;
  /** 並び順。省略すると既定値と同じ鍵になる。 */
  sort?: VideoSort;
  /** sort=random の並びを決める値。ほかの並び順では鍵に入れない。 */
  seed?: number;
  /**
   * フォルダ画面のフォルダ（`folderKey` の値）。ライブラリ一覧では省く。
   * 省いた鍵とフォルダを含む鍵は、検索語と並び順が同じでも一致しない。
   */
  folder?: string;
  /**
   * ライブラリのタグ絞り込み（id の並び。順は問わない、鍵の正規化がそろえる）。
   * `/?tag=1` から戻って `/` の控えが使われないようにする
   * （specs/014-video-tags/contracts/list-url.md §1）。
   */
  tags?: readonly number[];
}

/** ListSnapshot は一覧を離れる直前の状態である（data-model.md 2.）。 */
export interface ListSnapshot {
  /** 鍵。ListKey を正規化して連結したもの。 */
  key: string;
  /** 読み込み済みの項目（ページをまたいで連結済み）。 */
  items: Video[];
  /** 総件数（件数の表示に使う）。 */
  total: number;
  /** 次のページの続き位置。 */
  cursor?: string;
  /** 次のページがあるか。 */
  hasMore: boolean;
  /** 離れる直前のスクロール位置。 */
  scrollY: number;
  /** フォルダ画面では、直下の子フォルダの一覧も控える（戻ったときに往復しない）。 */
  folderListing?: FolderListing;
  /**
   * 控えを取った時点で分かっていた「最後に終わった取り込み」の id。
   *
   * 戻ってきたときにこれと違う取り込みが終わっていれば、控えの一覧は
   * 古い（動画が増減している）。同じなら控えをそのまま使ってよい。
   * 一度も取り込みを観測していなければ未設定。
   */
  scanId?: number;
}

/**
 * held は保持している控えである。
 *
 * **直近の 1 件だけ**を持つ。鍵ごとに溜めると、検索語を変えて回っただけで
 * 数千件の Video がメモリに残る（data-model.md 2.）。
 *
 * localStorage・sessionStorage には書かない。1 ページ 60 件 × 複数ページの
 * JSON を毎回直列化する費用に対し、得られるのは「タブを閉じても戻れる」ことだけで、
 * FR-016 はそれを求めていない。
 */
let held: ListSnapshot | undefined;

/**
 * normalize は鍵を文字列にする。
 *
 * 区切りに NUL を使うのは、検索語に何が入っていても `sort` との境界が
 * 曖昧にならないようにするためである（URL のクエリに NUL は現れない）。
 */
function normalize(key: ListKey): string {
  const sort = key.sort ?? defaultSort;
  const seed = sort === "random" ? String(key.seed ?? "") : "";
  const tags = [...(key.tags ?? [])].sort((a, b) => a - b).join(",");
  const list = [
    key.query.trim(),
    key.watch ?? "all",
    key.playable === true ? "1" : "",
    sort,
    seed,
    tags,
  ].join("\0");
  return key.folder === undefined ? list : `folder\0${key.folder}\0${list}`;
}

/** saveListSnapshot は一覧を離れる瞬間の状態を控える。前の控えは捨てる。 */
export function saveListSnapshot(key: ListKey, value: Omit<ListSnapshot, "key">): void {
  held = { key: normalize(key), ...value };
}

/**
 * takeListSnapshot は鍵の一致する控えを返す。**鍵が違えば undefined を返す。**
 * 復元できないことは異常ではなく、呼び出し側は 1 ページ目から読めばよい。
 *
 * 取り出しても控えは残す。React は開発時に効果を 2 回走らせるし、戻る操作が
 * 連続することもある — 1 回目で消してしまうと、そのどちらでも復元が消える。
 * 要らなくなった控えは次の保存が上書きするか、clearListSnapshot が捨てる。
 */
export function takeListSnapshot(key: ListKey): ListSnapshot | undefined {
  return held !== undefined && held.key === normalize(key) ? held : undefined;
}

/**
 * applyProgressToListSnapshot は控えの中の動画1件の再生位置を差し替える。
 * 再生画面から戻ったとき、見終えた動画を未視聴のまま復元しないためである。
 */
export function applyProgressToListSnapshot(videoId: number, progress: Progress): void {
  if (held === undefined || !held.items.some((video) => video.id === videoId)) return;
  held = {
    ...held,
    items: held.items.map((video) =>
      video.id === videoId ? { ...video, progress } : video,
    ),
  };
}

/**
 * applyVisibilityToListSnapshot は控えの中の動画たちの公開フラグを差し替える。
 * 公開・非公開の切り替えの直後に、控えを取り直さず結果を反映するために使う
 * （タグの付け外しの applyTagToListSnapshot と同じ扱い）。
 */
export function applyVisibilityToListSnapshot(
  videoIds: readonly number[],
  isPublic: boolean,
): void {
  if (held === undefined) return;
  const targets = new Set(videoIds);
  if (!held.items.some((video) => targets.has(video.id) && video.public !== isPublic))
    return;
  held = {
    ...held,
    items: held.items.map((video) =>
      targets.has(video.id) ? { ...video, public: isPublic } : video,
    ),
  };
}

/**
 * clearListSnapshot は控えを捨てる。取り込みが終わって一覧を読み直すときに
 * 呼ぶ — 取り込む前の一覧に戻してはならない（data-model.md 2.）。
 */
export function clearListSnapshot(): void {
  held = undefined;
}

/**
 * applyTagToListSnapshot は控えの中の動画たちのタグを書き換える。付け外しの
 * 直後に、控えを取り直さず結果を反映するために使う（issue 267、Plan の Structural
 * Decisions 7）。action = "remove" で絞り込みに合わなくなった項目も、その場では
 * 一覧から外さない（次の読み込みで反映する。contracts/tags-api.md §5）。
 */
export function applyTagToListSnapshot(
  videoIds: readonly number[],
  tag: TagRef,
  action: "add" | "remove",
): void {
  if (held === undefined) return;
  const targets = new Set(videoIds);
  if (!held.items.some((video) => targets.has(video.id))) return;
  held = {
    ...held,
    items: held.items.map((video) =>
      targets.has(video.id)
        ? { ...video, tags: applyTagToTags(video.tags, tag, action) }
        : video,
    ),
  };
}
