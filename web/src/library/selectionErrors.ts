import { RequestFailed } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import { t, type UiText } from "../i18n";

export function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

/**
 * `POST /api/video-tags`・`POST /api/video-tags/summary` は videoIds を全部か
 * 無しかでしか受け付けず、maxVideoTagsSelection を超えると400になる。トリガのボタンを disabled にするだけでは、
 * ポップオーバーを開いたまま選択が増える（例:「すべて選択」の応答が届く）と
 * 送れてしまうため、ポップオーバー自身も閉じ・その場の送信も selectedIds の
 * 最新の件数で確かめて理由を示す（Devin の指摘）。
 *
 * 公開の一括の切り替え（`PUT /api/video-visibility`）も同じ上限・同じ全部か無しかで
 * 、同じ理由を添える。そのため文言はタグに限らない言い方にする。
 * サーバーの too_many_videos と同じ文を使う。
 */
export function overLimitMessage(): UiText {
  return t.errors.reason.too_many_videos({ limit: maxVideoTagsSelection });
}
