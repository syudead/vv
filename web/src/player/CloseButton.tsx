import { X } from "lucide-react";

import { cn } from "../lib/cn";
import IconButton from "../ui/IconButton";

/**
 * CloseButton は画面を閉じる × である（要件 3、ui-design「Close」）。
 *
 * 幅ごとに 1 つずつ DOM に置き、どちらを見せるかは CSS だけで決める。
 * - `wide`：`lg` 以上。関連動画の見出しの行の右端。背景の面を持たない。
 * - `overlay`：`lg` 未満。プレイヤーの右上に丸い面付きで重ねる。`visible` が偽の間
 *   （再生中で操作バーが隠れている間）は見えなくする。
 */
export default function CloseButton({
  variant,
  onClose,
  visible = true,
}: {
  variant: "wide" | "overlay";
  onClose: () => void;
  visible?: boolean;
}) {
  if (variant === "wide") {
    return (
      <span className="hidden lg:block">
        <IconButton
          label="閉じる"
          onClick={onClose}
          className="text-fg-muted! hover:bg-hover-wash hover:text-fg! [&>svg]:size-5!"
        >
          <X aria-hidden="true" />
        </IconButton>
      </span>
    );
  }
  return (
    <span
      data-close-overlay=""
      className={cn(
        "absolute top-2 right-2 z-30 transition-opacity duration-150 motion-reduce:transition-none lg:hidden",
        // 見えない間は押せなくする。キーボードでフォーカスが来たときだけ見せる。
        visible
          ? "opacity-100"
          : "pointer-events-none opacity-0 focus-within:opacity-100",
      )}
    >
      <IconButton
        label="閉じる"
        onClick={onClose}
        className="h-11! w-11! rounded-full! bg-overlay! hover:bg-overlay! [&>svg]:size-5!"
      >
        <X aria-hidden="true" />
      </IconButton>
    </span>
  );
}
