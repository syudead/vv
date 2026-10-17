import { Clapperboard } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link } from "react-router";

import type { Video } from "../../api/client";
import { t } from "../../i18n";
import { videoEditPath } from "../../lib/videoEditPath";
import ActionButton from "../ActionButton";
import { handVideoToEditor } from "./handoff";

/** DetailsLinkState は、編集画面へ載せる動画ページの戻り先である。 */
export interface DetailsLinkState {
  /** 動画ページの × の戻り先（一覧）。 */
  from: string;
  /** 動画ページへ来るまでに見てきた動画。 */
  trail: number[];
}

/**
 * DetailsEntry は情報の行の操作群に置く、作品情報と表示名の編集画面への入口である。router の Link で
 * `/videos/:id/edit` を開き、ページの戻り先（`from`・`trail`）を載せる。ページの `Video` は
 * 控えで渡し、編集画面はそれから始める。編集画面から戻ったときはここへフォーカスを置く。
 */
export interface DetailsEntryProps {
  video: Video;
  /** 所有者のときだけ置く。 */
  owner: boolean;
  /** 載せる戻り先と、置いたときにフォーカスを移すか。無ければ置かない。 */
  details?: { linkState: DetailsLinkState; focusEntry: boolean };
}

export default function DetailsEntry({ video, owner, details }: DetailsEntryProps) {
  const ref = useRef<HTMLAnchorElement | null>(null);
  const focused = useRef(false);
  const focusEntry = details?.focusEntry === true;

  useEffect(() => {
    if (!focusEntry || focused.current) return;
    focused.current = true;
    ref.current?.focus();
  }, [focusEntry]);

  if (!owner || details === undefined) return null;
  return (
    <ActionButton label={t.player.workInfo.edit} asChild>
      <Link
        ref={ref}
        to={videoEditPath(video.id)}
        state={{ ...details.linkState, fromPage: true }}
        onClick={() => handVideoToEditor(video)}
      >
        <Clapperboard aria-hidden="true" />
      </Link>
    </ActionButton>
  );
}
