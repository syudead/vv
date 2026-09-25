import { LoaderCircle } from "lucide-react";
import { useRef } from "react";

import type { Tag } from "../api/tags";
import Button from "../ui/Button";
import { ModalFrame } from "../ui/ModalFrame";

/**
 * DeleteTagDialog はタグ削除の確認の窓である（ui-design.md「Merge and
 * delete」、受け入れ条件 11）。本数は `GET /api/tags` の `videoCount`。
 */
export default function DeleteTagDialog({
  tag,
  pending,
  error,
  onClose,
  onDelete,
}: {
  tag: Tag;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onDelete: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  const message =
    tag.videoCount === 0
      ? "このタグはどの動画にも付いていません。"
      : `${String(tag.videoCount)} 本の動画からこのタグが外れます。この操作は取り消せません。`;

  return (
    <ModalFrame title={`「${tag.name}」を削除`} onClose={onClose} initialFocus={cancel}>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
        <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
          {message}
        </p>
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={onClose} disabled={pending}>
          キャンセル
        </Button>
        <Button variant="danger" onClick={onDelete} disabled={pending}>
          {pending && <LoaderCircle className="animate-spin" />}
          削除する
        </Button>
      </div>
    </ModalFrame>
  );
}
