import { fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useKeyboardShortcuts } from "./keyboard";
import type { PlayerControls } from "./playerControls";

function fakeControls(overrides: Partial<PlayerControls> = {}): PlayerControls {
  return {
    togglePlay: vi.fn(),
    play: vi.fn(),
    seekBy: vi.fn(),
    seekTo: vi.fn(),
    restart: vi.fn(),
    toggleMute: vi.fn(),
    toggleFullscreen: vi.fn(),
    isFullscreen: vi.fn(() => false),
    menuOpen: vi.fn(() => false),
    wake: vi.fn(),
    ...overrides,
  };
}

function Harness({
  controls,
  onClose,
}: {
  controls: PlayerControls | null;
  onClose: () => void;
}) {
  useKeyboardShortcuts(controls, onClose);
  return (
    <div>
      <button type="button">ボタン</button>
      <a href="/videos/2">リンク</a>
      <input aria-label="入力" />
      <div role="slider" tabIndex={0} aria-label="再生位置" aria-valuenow={0} />
      <div data-testid="plain" tabIndex={-1} />
    </div>
  );
}

function setup(controls: PlayerControls | null = fakeControls()) {
  const onClose = vi.fn();
  const view = render(<Harness controls={controls} onClose={onClose} />);
  return { onClose, view, controls };
}

describe("useKeyboardShortcuts", () => {
  afterEach(() => {
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      value: null,
    });
  });

  it("Space・←/→・F・M・0 がそれぞれの操作を起こす", () => {
    const controls = fakeControls();
    setup(controls);
    const target = document.body;
    fireEvent.keyDown(target, { key: " " });
    fireEvent.keyDown(target, { key: "ArrowLeft" });
    fireEvent.keyDown(target, { key: "ArrowRight" });
    fireEvent.keyDown(target, { key: "f" });
    fireEvent.keyDown(target, { key: "M" });
    fireEvent.keyDown(target, { key: "0" });

    expect(controls.togglePlay).toHaveBeenCalledTimes(1);
    expect(controls.seekBy).toHaveBeenNthCalledWith(1, -10);
    expect(controls.seekBy).toHaveBeenNthCalledWith(2, 10);
    expect(controls.toggleFullscreen).toHaveBeenCalledTimes(1);
    expect(controls.toggleMute).toHaveBeenCalledTimes(1);
    expect(controls.seekTo).toHaveBeenCalledWith(0);
  });

  it("操作バーの部品が伝播を止めても、捕捉段階で受ける", () => {
    const controls = fakeControls();
    const { view } = setup(controls);
    const plain = view.getByTestId("plain");
    plain.addEventListener("keydown", (event) => event.stopPropagation());
    fireEvent.keyDown(plain, { key: "ArrowRight" });
    expect(controls.seekBy).toHaveBeenCalledWith(10);
  });

  it("ボタンやリンクの上の Space では起きない", () => {
    const controls = fakeControls();
    const { view } = setup(controls);
    fireEvent.keyDown(view.getByRole("button"), { key: " " });
    fireEvent.keyDown(view.getByRole("link"), { key: " " });
    expect(controls.togglePlay).not.toHaveBeenCalled();
    // ボタンの上でも Space 以外のキーは効く。
    fireEvent.keyDown(view.getByRole("button"), { key: "ArrowRight" });
    expect(controls.seekBy).toHaveBeenCalledWith(10);
  });

  it("入力欄では起きず、スライダーの上の ← → はスライダー自身に任せる", () => {
    const controls = fakeControls();
    const { view, onClose } = setup(controls);
    const input = view.getByRole("textbox");
    for (const key of [" ", "ArrowRight", "f", "m", "0", "Escape"]) {
      fireEvent.keyDown(input, { key });
    }
    fireEvent.keyDown(view.getByRole("slider"), { key: "ArrowRight" });
    expect(controls.togglePlay).not.toHaveBeenCalled();
    expect(controls.seekBy).not.toHaveBeenCalled();
    expect(controls.toggleFullscreen).not.toHaveBeenCalled();
    expect(controls.toggleMute).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("修飾キー付きでは起きない", () => {
    const controls = fakeControls();
    setup(controls);
    fireEvent.keyDown(document.body, { key: "f", ctrlKey: true });
    fireEvent.keyDown(document.body, { key: "ArrowRight", altKey: true });
    fireEvent.keyDown(document.body, { key: "m", metaKey: true });
    expect(controls.toggleFullscreen).not.toHaveBeenCalled();
    expect(controls.seekBy).not.toHaveBeenCalled();
    expect(controls.toggleMute).not.toHaveBeenCalled();
  });

  it("Esc で閉じる。プレイヤーが無くても閉じる", () => {
    const { onClose } = setup(null);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("全画面中の Esc では閉じない", () => {
    const controls = fakeControls({ isFullscreen: vi.fn(() => true) });
    const { onClose } = setup(controls);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();

    const other = setup(fakeControls());
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      value: document.body,
    });
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(other.onClose).not.toHaveBeenCalled();
  });

  it("再生速度のメニューが開いているときの Esc はメニューに任せる", () => {
    const controls = fakeControls({ menuOpen: vi.fn(() => true) });
    const { onClose } = setup(controls);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
  });

  it("画面の外の部品が開いた吹き出し（取り込み状況の概要など）の Esc では閉じない", () => {
    const { onClose } = setup(null);
    const wrapper = document.createElement("div");
    wrapper.setAttribute("data-radix-popper-content-wrapper", "");
    const content = document.createElement("div");
    content.setAttribute("role", "dialog");
    wrapper.append(content);
    document.body.append(wrapper);
    try {
      fireEvent.keyDown(content, { key: "Escape" });
      expect(onClose).not.toHaveBeenCalled();

      // ツールチップは吹き出しに含めない。出ていても Esc で閉じる。
      content.setAttribute("role", "tooltip");
      fireEvent.keyDown(document.body, { key: "Escape" });
      expect(onClose).toHaveBeenCalledTimes(1);
    } finally {
      wrapper.remove();
    }
  });

  it("プレイヤーが無いときは再生の操作をしない", () => {
    setup(null);
    expect(() => fireEvent.keyDown(document.body, { key: " " })).not.toThrow();
  });
});
