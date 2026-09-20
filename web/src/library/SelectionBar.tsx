import { X } from "lucide-react";

import Button from "../ui/Button";
import IconButton from "../ui/IconButton";

/** SelectionBar は 1 件以上選ぶと画面下部に浮く。 */
export default function SelectionBar({
  count,
  total,
  onSelectAll,
  onClear,
}: {
  count: number;
  total: number;
  onSelectAll: () => void;
  onClear: () => void;
}) {
  if (count === 0) return null;

  return (
    <div
      role="region"
      aria-label="選択中の操作"
      className="fixed inset-x-0 bottom-5 z-30 flex justify-center px-4"
    >
      <div className="flex h-12 items-center gap-2 rounded-xl bg-elevated pr-1.5 pl-4 shadow-elevated animate-slide-up">
        <span className="text-sm font-medium text-fg tabular-nums">
          {count.toLocaleString("ja-JP")} 件を選択中
        </span>
        <span className="mx-1 h-5 w-px bg-border" />
        <Button variant="ghost" size="sm" onClick={onSelectAll} disabled={count >= total}>
          すべて選択
        </Button>
        <IconButton label="選択を解除 (Esc)" size="sm" onClick={onClear}>
          <X />
        </IconButton>
      </div>
    </div>
  );
}
