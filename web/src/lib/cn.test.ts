import { cn } from "./cn";

describe("cn", () => {
  it("keeps the font size and the colour of vv's text classes", () => {
    expect(cn("text-sm", "text-fg-muted")).toBe("text-sm text-fg-muted");
    expect(cn("text-xs", "text-danger")).toBe("text-xs text-danger");
  });

  it("keeps different vv colour roles on different properties", () => {
    expect(cn("bg-surface", "text-fg", "border-border")).toBe(
      "bg-surface text-fg border-border",
    );
  });
});
