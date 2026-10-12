import { ImageUp, LoaderCircle } from "lucide-react";
import { type ChangeEvent, useEffect, useRef, useState } from "react";

import {
  isAborted,
  RequestFailed,
  uploadThumbnailImage,
  type Video,
} from "../../api/client";
import { detailMark, type DetailMark } from "../../api/useVideoDetail";
import { errorText, t, type UiText } from "../../i18n";
import { Input } from "../../ui/shadcn/input";
import ActionButton from "../ActionButton";
import { UPLOAD_TYPES, uploadRefusal } from "./uploadRules";

/**
 * useThumbnailUpload は選んだ画像を動画のサムネイルとして送る
 * （specs/046-thumbnail-image-upload/ui-design.md「Video page: upload entry」）。
 *
 * 送る前に種類と大きさを確かめ、断るときは何も送らずにその文言を `report` へ渡す。送り始める
 * ときに `report(null)`、失敗したら `Couldn't upload the image: {reason}` を渡し、
 * `video_not_found`・`file_unavailable`（`404`）と `content_changed` では `onStale` で動画を
 * 取り直させる。成功したら応答の動画を、送る直前に取った detailMark と一緒に `onChanged` で渡す。
 *
 * ページを離れても送信は止めない。本文が届けばサーバーが適用を終える（R-1）。離れた後の答えは
 * 画面に反映しない。
 */
export function useThumbnailUpload(
  videoId: number,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): { sending: boolean; upload: (file: File) => void } {
  const [sending, setSending] = useState(false);
  // 同じ瞬間の二度押しも止めるため、状態とは別に持つ。
  const busy = useRef(false);
  const live = useRef(true);

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  const upload = (file: File) => {
    if (busy.current) return;
    const text = t.player.thumbnailUpload;
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      report(refusal === "type" ? text.refusedType : text.refusedSize);
      return;
    }
    busy.current = true;
    const mark = detailMark();
    report(null);
    setSending(true);
    uploadThumbnailImage(videoId, file)
      .then(
        (video) => {
          if (live.current) onChanged?.(video, mark);
        },
        (error: unknown) => {
          if (isAborted(error) || !live.current) return;
          report(text.failed(errorText(error)));
          if (staleAfter(error)) onStale?.();
        },
      )
      .finally(() => {
        busy.current = false;
        if (live.current) setSending(false);
      });
  };

  return { sending, upload };
}

/** staleAfter は、失敗の後にページが動画を取り直すべきか（029 と同じ）を返す。 */
function staleAfter(error: unknown): boolean {
  if (!(error instanceof RequestFailed)) return false;
  return error.status === 404 || error.reason === "content_changed";
}

/**
 * UploadEntry は情報の行の右端の「Upload an image as thumbnail」である。押すと PNG・JPEG・
 * WebP に限ったファイルピッカーを開き、選んだファイルを `onFile` へ渡す。同じファイルを続けて
 * 選べるよう、選ぶたびに入力を空にする。送信中は回り、送信中か `locked`（ほかのサムネイルの
 * 操作の送信中）の間は `aria-disabled` で、押してもピッカーを開かない。
 */
export default function UploadEntry({
  sending,
  locked,
  onFile,
}: {
  sending: boolean;
  locked: boolean;
  onFile: (file: File) => void;
}) {
  const fileInput = useRef<HTMLInputElement | null>(null);
  const disabled = sending || locked;

  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file !== undefined) onFile(file);
  };

  return (
    <>
      <ActionButton
        label={t.player.thumbnailUpload.entry}
        aria-disabled={disabled || undefined}
        onClick={() => {
          if (!disabled) fileInput.current?.click();
        }}
        className="aria-disabled:cursor-default"
      >
        {sending ? (
          <LoaderCircle
            aria-hidden="true"
            className="animate-spin motion-reduce:animate-none"
          />
        ) : (
          <ImageUp aria-hidden="true" />
        )}
      </ActionButton>
      <Input
        ref={fileInput}
        type="file"
        accept={UPLOAD_TYPES}
        tabIndex={-1}
        aria-hidden="true"
        className="hidden"
        data-testid="thumbnail-upload-input"
        onChange={choose}
      />
    </>
  );
}
