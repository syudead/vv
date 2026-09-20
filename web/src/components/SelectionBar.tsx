import Icon from "../layout/icons";

export default function SelectionBar({
  count,
  onClear,
}: {
  count: number;
  onClear: () => void;
}) {
  if (count === 0) {
    return null;
  }

  const clearAndRestoreFocus = () => {
    const firstSelected = document.querySelector<HTMLInputElement>(
      'input[data-video-select="true"]:checked',
    );
    onClear();
    requestAnimationFrame(() => firstSelected?.focus());
  };

  return (
    <div
      role="toolbar"
      aria-label="選択操作"
      className="selection-bar fixed bottom-4 z-40 flex items-center gap-2 rounded-card border border-border bg-surface-raised p-1.5 shadow-2xl"
    >
      <span
        role="status"
        aria-live="polite"
        className="min-w-20 px-2 text-center text-sm font-medium text-body tabular-nums"
      >
        {String(count)}件選択
      </span>
      <span aria-hidden className="h-6 w-px bg-border" />
      <button
        type="button"
        onClick={clearAndRestoreFocus}
        title="選択を解除"
        className="flex min-h-[var(--size-tap)] items-center gap-1.5 whitespace-nowrap rounded-control px-2.5 text-sm text-muted outline-none transition-colors hover:bg-body/10 hover:text-body active:bg-surface-sunken active:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus motion-reduce:transition-none"
      >
        <Icon name="close" className="h-4 w-4" />
        <span>選択解除</span>
      </button>
    </div>
  );
}
