import { Layers } from "lucide-react";

import type { Video } from "../api/client";
import { t } from "../i18n";

// 束ねた動画のカードと行の長さの印に添える、バージョンがあることの印
// （specs/054-auto-dup/ui-design.md「Versions mark」）。

/**
 * shownVersionsCount は印に出すバージョンの本数である。集まりのメンバーで、見る人に見せる
 * バージョンが 2 本以上のときだけ数を返し、それ以外は null（動画のページの項目と同じ規則）。
 */
export function shownVersionsCount(video: Video): number | null {
  const count = video.versions?.count ?? 0;
  return count >= 2 ? count : null;
}

/**
 * VersionsMark は長さの印の先頭に置くバージョンの印（`Layers` と、読み上げ用の「3 versions」）
 * である。押せる物ではない。`id` はカードのリンクが `aria-describedby` で読み上げ用の文を指すのに使う。
 */
export function VersionsMark({ count, id }: { count: number; id?: string }) {
  return (
    <>
      <Layers aria-hidden="true" className="size-3 shrink-0" />
      <span id={id} className="sr-only">
        {t.player.versions.count(count)}
      </span>
    </>
  );
}
