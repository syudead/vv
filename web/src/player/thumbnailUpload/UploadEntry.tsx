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
import { encodeComposition, loadUploadImage, type UploadImage } from "./composeImage";
import { type Fit, frameAspect } from "./framing";
import { UPLOAD_TYPES, uploadRefusal } from "./uploadRules";

/** UploadPreviewState は開いているプレビューの中身である。 */
export interface UploadPreviewState {
  image: UploadImage;
  name: string;
  /** プレビューを開いたときの動画の比率。分からなければ `null`。 */
  aspect: number | null;
  /** プレビューを開いたときの動画の内容の版。適用はこの内容の動画にだけ効く。 */
  contentVersion: string;
}

/**
 * useThumbnailUpload は選んだ画像をプレビューで確かめてから動画のサムネイルとして送る
 * （docs/design-docs/thumbnail-editing.md「Uploaded images」）。
 *
 * `choose` は送る前に種類と大きさを確かめ、画像を読んで（見出しのアニメーション・画素数、復号）
 * プレビューを開く。断るときは何も送らずにその文言を `report` へ渡す。読んでいる間は `preparing`
 * である。`apply` はプレビューの形で描いた画像を、開いたときの `contentVersion` と一緒に送り、
 * `cancel` は何も送らずに閉じる。動画が別のものに替われば、プレビューは何も送らずに閉じる。
 *
 * 送り始めるときに `report(null)`、失敗したら `Couldn't upload the image: {reason}` を渡し、
 * `video_not_found`・`file_unavailable`（`404`）と `content_changed` では `onStale` で動画を
 * 取り直させる。成功したら応答の動画を、送る直前に取った detailMark と一緒に `onChanged` で渡す。
 *
 * ページを離れても送信は止めない。本文が届けばサーバーが適用を終える。離れた後の答えは
 * 画面に反映しない。
 */
export function useThumbnailUpload(
  video: Video,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: boolean;
  preparing: boolean;
  applying: boolean;
  preview: UploadPreviewState | null;
  choose: (file: File) => void;
  apply: (fit: Fit) => void;
  cancel: () => void;
} {
  const [sending, setSending] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [applying, setApplying] = useState(false);
  const [preview, setPreview] = useState<UploadPreviewState | null>(null);
  // 同じ瞬間の二度押しも止めるため、状態とは別に持つ。
  const busy = useRef(false);
  const live = useRef(true);
  // 読んでいる途中で動画が替わったら、その画像のプレビューは開かない。
  const videoId = useRef(video.id);
  videoId.current = video.id;

  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);

  // 別の動画へ移ったら、前の動画のプレビューは何も送らずに閉じる。
  useEffect(() => {
    setPreview((current) => {
      if (current !== null) {
        current.image.bitmap.close();
        busy.current = false;
      }
      return null;
    });
  }, [video.id]);

  const text = t.player.thumbnailUpload;

  const choose = (file: File) => {
    if (busy.current) return;
    const refusal = uploadRefusal(file);
    if (refusal !== null) {
      report(refusal === "type" ? text.refusedType : text.refusedSize);
      return;
    }
    busy.current = true;
    report(null);
    setPreparing(true);
    const opened = {
      id: video.id,
      aspect: frameAspect(video),
      contentVersion: video.contentVersion ?? "",
    };
    loadUploadImage(file)
      .then((loaded) => {
        if (!live.current || videoId.current !== opened.id) {
          if ("image" in loaded) loaded.image.bitmap.close();
          busy.current = false;
          return;
        }
        if ("refusal" in loaded) {
          report(text.failed(t.errors.reason[loaded.refusal]));
          busy.current = false;
          return;
        }
        setPreview({ image: loaded.image, name: file.name, ...opened });
      })
      .catch(() => {
        if (live.current) report(text.failed(t.errors.reason.thumbnail_upload_invalid));
        busy.current = false;
      })
      .finally(() => {
        if (live.current) setPreparing(false);
      });
  };

  const close = (current: UploadPreviewState) => {
    current.image.bitmap.close();
    setPreview(null);
  };

  const cancel = () => {
    if (preview === null || applying) return;
    close(preview);
    busy.current = false;
  };

  const apply = (fit: Fit) => {
    if (preview === null || applying) return;
    const current = preview;
    setApplying(true);
    encodeComposition(current.image, current.aspect, fit)
      .then((composed) => {
        if (!live.current) return;
        close(current);
        send(new File([composed], "thumbnail.png", { type: "image/png" }), current);
      })
      .catch(() => {
        if (!live.current) return;
        close(current);
        busy.current = false;
        report(text.failed(t.errors.unexpected));
      })
      .finally(() => {
        if (live.current) setApplying(false);
      });
  };

  const send = (image: File, current: UploadPreviewState) => {
    const mark = detailMark();
    setSending(true);
    uploadThumbnailImage(videoId.current, image, current.contentVersion)
      .then(
        (saved) => {
          if (live.current) onChanged?.(saved, mark);
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

  return { sending, preparing, applying, preview, choose, apply, cancel };
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
