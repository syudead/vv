import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import { CardDragLayer, DraggableCard } from "./cardDrag";

/** Not memoized, so it renders whenever its DraggableCard renders with a new element. */
function Face({ label, onRender }: { label: string; onRender: (label: string) => void }) {
  onRender(label);
  return <span>{label}</span>;
}

describe("DraggableCard", () => {
  it("skips loaded cards that receive equal fresh card elements", () => {
    const renders = new Set<string>();
    const count = (label: string) => renders.add(label);
    const list = (keys: readonly string[]) => (
      <CardDragLayer
        order={keys}
        liftedFor={(key) => [key]}
        reorder={false}
        nameOf={(key) => key}
        preview={(key) => key}
        onDrop={vi.fn()}
        instructions={t.organize.instructions}
      >
        {keys.map((key) => (
          <DraggableCard key={key} id={key}>
            <Face label={key} onRender={count} />
          </DraggableCard>
        ))}
      </CardDragLayer>
    );
    const { rerender } = render(list(["a", "b"]));
    renders.clear();

    rerender(list(["a", "b", "c"]));
    expect(renders).toEqual(new Set(["c"]));
  });
});
