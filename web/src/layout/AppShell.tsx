import {
  createContext,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
  useContext,
  useEffect,
  useState,
} from "react";

import Header from "./Header";
import Sidebar from "./Sidebar";

/**
 * AppShell は 3 領域の骨格である（FR-001 / FR-002 / contracts/layout.md 1.）。
 *
 * 左にサイドバー（C1）、上にヘッダー（C5）、残りが子（コンテンツ）である。
 * 骨格は**どの画面にも同じ形で存在し、画面の中身を知らない** ── どの画面を
 * 包むかを決めるのは `App.tsx` の経路の側である（FR-015。再生画面は包まない）。
 *
 * **内側にスクロール容器を作らない。** サイドバーとヘッダーは `fixed` で画面に
 * 固定されているので、コンテンツはその分を `padding` で避けるだけでよく、
 * スクロールの持ち主は文書（ウィンドウ）のままである（R-501）。ここに
 * `overflow-y: auto` の器を置くと、004 の復元・密度アンカー・無限スクロールが
 * 3 つとも書き換えになる。
 *
 * 重なりの順序はサイドバー・ヘッダー（z-30）がコンテンツより前、ツールバー
 * （z-20）がヘッダーより後ろである（contracts/layout.md 1.）。
 *
 * **画面を満たす役目はここが持つ**（`min-h-dvh`）。包む側と包まれる側の両方が
 * 100dvh を要求すると、文書の最低の高さが `100dvh + --size-header` になり、
 * 件数が少ない一覧でヘッダーの高さぶん余計に縦スクロールできてしまう。
 * `box-sizing: border-box` なので、この 1 か所に置けば余白も 100dvh の内側に
 * 収まる。
 */

/**
 * 件数は骨格の中を**子から親へ**流れる（T021）。
 *
 * 総件数を持っているのは `useVideos` を呼ぶ `LibraryPage` で、それは
 * AppShell の**子**である。出す先の NavItem「すべての動画」は Sidebar の中、
 * つまり AppShell の別の枝にある ── 引数では渡せないので、骨格の中に置いた
 * この状態を経由させる。
 *
 * **新しいファイルを作らない。** この状態は骨格の外で使われないので、器
 * （AppShell）と同じファイルに置くほうが、どこまでが骨格の関心かが読み取れる。
 *
 * 値と設定関数を**別の context に分ける**のは、設定関数の同一性を保つためで
 * ある。1 つのオブジェクトにまとめると、件数が変わるたびに新しいオブジェクトに
 * なり、公開側の効果が毎回走り直す。
 */
const CountContext = createContext<number | undefined>(undefined);
const SetCountContext = createContext<Dispatch<SetStateAction<number | undefined>>>(
  () => undefined,
);

/**
 * useVideoCount は一覧の総件数を読む（NavItem「すべての動画」の右端）。
 *
 * **`undefined` は「まだ分からない」である。** 骨格の外（再生画面やテスト）で
 * 呼ばれたときも、一覧がまだ読めていないときも `undefined` になる。どちらも
 * 件数を出さない場面なので、読み出し側は 1 つの分岐で足りる。
 */
export function useVideoCount(): number | undefined {
  return useContext(CountContext);
}

/**
 * usePublishVideoCount は一覧の総件数を骨格へ公開する。
 *
 * 呼び出し側（LibraryPage）は**公開するだけ**で、どこにどう出るかを知らない。
 *
 * 一覧を離れたら取り消す（後始末で `undefined` に戻す）。骨格ごと消える経路
 * （`/videos/:id` は包まれていない）では要らないが、骨格の中で一覧以外を描く
 * ようになったときに古い件数が残るのを、ここで先に塞いでおく。
 */
export function usePublishVideoCount(count: number | undefined): void {
  const setCount = useContext(SetCountContext);

  useEffect(() => {
    setCount(count);
  }, [setCount, count]);

  useEffect(() => {
    return () => {
      setCount(undefined);
    };
  }, [setCount]);
}

export default function AppShell({ children }: { children: ReactNode }) {
  const [count, setCount] = useState<number | undefined>(undefined);

  return (
    <SetCountContext value={setCount}>
      <CountContext value={count}>
        <div className="min-h-dvh pt-[var(--size-header)] sm:pl-[var(--size-sidebar)]">
          <Sidebar />
          <Header />
          {children}
        </div>
      </CountContext>
    </SetCountContext>
  );
}
