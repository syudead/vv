import { AlertCircle, Globe, LoaderCircle, Lock } from "lucide-react";
import { useState } from "react";

import { updateVideoVisibility } from "../api/visibility";
import { cn } from "../lib/cn";

/**
 * VisibilitySwitch は再生画面の公開の切り替えである
 * （specs/016-single-account-auth/ui-design.md「Visibility toggle」の「Video page」）。
 *
 * 押すと `PUT /api/video-visibility` を1回送り、応答を受けてから状態を変える。
 * 状態は親（useVideoDetail）の `public` が持ち、応答の通知（api/visibility.ts）で
 * 差し替わる。トーストは出さない。部品自身の文言が変わるので、それが結果である。
 *
 * 送信中は `disabled` 属性ではなく `aria-disabled` にする。`disabled` にすると
 * ブラウザがフォーカスを外し、キーボードで押した人が応答の後にもう一度 Space で
 * 押せなくなる（ui-design.md のキーボード確認の手順 4）。押しても送らないのは
 * `toggle` が `sending` で確かめる。
 *
 * 失敗の行は次に押したときに消える。別の動画へ移ったときに消えるよう、呼び出し側が
 * 動画の id を `key` にして作り直す。
 */
export default function VisibilitySwitch({
  videoId,
  isPublic,
}: {
  videoId: number;
  isPublic: boolean;
}) {
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState(false);

  function toggle() {
    if (sending) return;
    setFailed(false);
    setSending(true);
    updateVideoVisibility([videoId], !isPublic)
      .catch(() => setFailed(true))
      .finally(() => setSending(false));
  }

  const Icon = sending ? LoaderCircle : isPublic ? Globe : Lock;

  return (
    <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:gap-3">
      <button
        type="button"
        role="switch"
        aria-checked={isPublic}
        aria-label="ログインしていない人に公開する"
        aria-disabled={sending || undefined}
        onClick={toggle}
        className={cn(
          "inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-md px-2.5 text-xs font-medium whitespace-nowrap transition-colors duration-150 select-none aria-disabled:cursor-default aria-disabled:opacity-50",
          isPublic
            ? "bg-accent-soft text-link hover:text-fg"
            : "bg-elevated text-fg hover:bg-hover-wash hover:bg-blend-lighten active:bg-active-wash",
        )}
      >
        <Icon
          aria-hidden="true"
          className={cn("size-4", sending && "animate-spin motion-reduce:animate-none")}
        />
        {isPublic ? "公開中" : "非公開"}
      </button>
      {failed && (
        <p role="alert" className="flex items-center gap-1.5 text-sm text-danger">
          <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
          変更できませんでした
        </p>
      )}
    </div>
  );
}
