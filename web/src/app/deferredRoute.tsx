import { AlertCircle } from "lucide-react";
import { type ComponentType, useEffect, useState } from "react";

import Button from "../ui/Button";
import { EmptyState } from "../videoList/states";

type PageModule = { default: ComponentType };

/** matchesPath は pathname が path そのものか、その下の場所かを返す。 */
function matchesPath(pathname: string, path: string) {
  return pathname === path || pathname.startsWith(`${path}/`);
}

/** RouteLoadFailed は画面の部品を読み込めなかったときに画面の代わりに出す。 */
export function RouteLoadFailed({ onReload }: { onReload: () => void }) {
  return (
    <EmptyState
      icon={AlertCircle}
      tone="danger"
      title="画面を読み込めませんでした"
      description="ページを再読み込みしてください。"
      action={<Button onClick={onReload}>再読み込み</Button>}
    />
  );
}

/**
 * deferredRoute は、画面の部品を使うときにだけ読み込む route の component を返す。
 *
 * ページを読み込むたびに App の import 先はすべて取り直されるので、ほかの画面の
 * 部品を一覧や再生画面の読み込みに加えない。起動したときの URL が path に当たる
 * ときは、読み終えてから返す。最初の描画から画面とキー操作がそろう（React.lazy
 * だと Suspense の表示の間引きで数百ミリ秒遅れる）。ただし、この待ちはページの
 * load イベントを遅らせないので、直接開いたときに画面が出るのは、import した
 * ときより数十ミリ秒遅れることがある。アプリの中で移ってきたときは、読み終えたところで
 * 画面を出す。読み込めなければ、空白にせず再読み込みを促す。
 */
export async function deferredRoute(
  load: () => Promise<PageModule>,
  path: string,
  reload: () => void = () => window.location.reload(),
): Promise<ComponentType> {
  let loaded: ComponentType | undefined;
  const loadPage = async () => {
    loaded = (await load()).default;
    return loaded;
  };
  if (matchesPath(window.location.pathname, path)) {
    // 失敗したときは描画の側で読み直し、それでも駄目なら失敗の表示を出す。
    await loadPage().catch(() => undefined);
  }

  return function DeferredRoute() {
    const [Page, setPage] = useState(() => loaded);
    const [failed, setFailed] = useState(false);
    useEffect(() => {
      if (Page !== undefined) return;
      let live = true;
      loadPage().then(
        (page) => {
          if (live) setPage(() => page);
        },
        () => {
          if (live) setFailed(true);
        },
      );
      return () => {
        live = false;
      };
    }, [Page]);

    if (Page !== undefined) return <Page />;
    if (failed) return <RouteLoadFailed onReload={reload} />;
    return null;
  };
}
