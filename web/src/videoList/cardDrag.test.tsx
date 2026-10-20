import type { UseDraggableArguments } from "@dnd-kit/core";
import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import { CardDragLayer, DraggableCard } from "./cardDrag";

const registered = vi.hoisted(() => [] as string[]);

// Each DraggableCard render registers its card once, so the ids show which cards rendered.
vi.mock("@dnd-kit/core", async (importOriginal) => {
  const core = await importOriginal<typeof import("@dnd-kit/core")>();
  return {
    ...core,
    useDraggable: (args: UseDraggableArguments) => {
      registered.push(String(args.id));
      return core.useDraggable(args);
    },
  };
});

function list(keys: readonly string[]) {
  return (
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
          <span>{key}</span>
        </DraggableCard>
      ))}
    </CardDragLayer>
  );
}

describe("DraggableCard", () => {
  it("renders only the added card when the list grows", () => {
    const { rerender } = render(list(["a", "b"]));
    registered.length = 0;

    rerender(list(["a", "b", "c"]));
    expect(new Set(registered)).toEqual(new Set(["c"]));
  });
});
