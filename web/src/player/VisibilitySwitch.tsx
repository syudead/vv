import { AlertCircle, Globe, LoaderCircle, Lock } from "lucide-react";
import { useId, useState } from "react";

import { updateVideoVisibility } from "../api/visibility";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Field } from "../ui/shadcn/field";
import { Label } from "../ui/shadcn/label";
import { Switch } from "../ui/shadcn/switch";

/**
 * VisibilitySwitch は再生画面の公開の切り替えである
 * （specs/016-single-account-auth/ui-design.md「Visibility toggle」の「Video page」）。
 * すぐ効く設定なので `Switch` にし、今の状態（公開中・非公開）を印と一緒に `Label` で添える
 * （web/registry/rules/components.md「Checkbox and Switch」）。
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
  onChanged,
}: {
  videoId: number;
  isPublic: boolean;
  /**
   * 切り替えが成功したときに呼ぶ。更新日時が進むので、呼び出し側が動画を取り直す
   * （specs/033-video-dates/ui-design.md「Refresh after edits」）。
   */
  onChanged?: () => void;
}) {
  const [sending, setSending] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);

  function toggle() {
    if (sending) return;
    setFailure(null);
    setSending(true);
    updateVideoVisibility([videoId], !isPublic)
      .then(() => onChanged?.())
      .catch((error: unknown) => setFailure(t.player.visibility.failed(errorText(error))))
      .finally(() => setSending(false));
  }

  const Icon = sending ? LoaderCircle : isPublic ? Globe : Lock;
  const id = useId();

  return (
    <div className="flex flex-col items-start gap-1 sm:flex-row sm:items-center sm:gap-3">
      <Field orientation="horizontal" className="w-auto">
        <Switch
          id={id}
          checked={isPublic}
          aria-label={t.player.visibility.label}
          aria-disabled={sending || undefined}
          onCheckedChange={toggle}
          className="aria-disabled:cursor-default aria-disabled:opacity-50"
        />
        <Label
          htmlFor={id}
          className={cn("text-base font-normal", isPublic && "text-primary")}
        >
          <Icon
            aria-hidden="true"
            className={cn("size-4", sending && "animate-spin motion-reduce:animate-none")}
          />
          {isPublic ? t.player.visibility.public : t.player.visibility.private}
        </Label>
      </Field>
      {failure !== null && (
        <p role="alert" className="flex items-center gap-1.5 text-base text-destructive">
          <AlertCircle aria-hidden="true" className="size-4 shrink-0" />
          {failure}
        </p>
      )}
    </div>
  );
}
