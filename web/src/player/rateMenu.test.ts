import videojs from "video.js";
import { afterEach, describe, expect, it } from "vitest";

import { rateMenuOpen } from "./playerControls";

/**
 * 再生速度のメニューが開いているかを、本物の video.js が作る DOM の印で確かめる。
 * video.js を上げて印の名前が変わると、ここが落ちて気付ける（Esc で画面を閉じてしまう）。
 */
describe("rateMenuOpen", () => {
  let player: ReturnType<typeof videojs> | undefined;

  afterEach(() => {
    player?.dispose();
    player = undefined;
  });

  function create() {
    const element = document.createElement("video-js");
    document.body.append(element);
    player = videojs(element, {
      controls: true,
      playbackRates: [0.5, 1, 2],
      controlBar: { children: ["playToggle", "playbackRateMenuButton"] },
    });
    const host = player.el();
    const button = host.querySelector<HTMLElement>(".vjs-playback-rate");
    if (button === null) throw new Error("再生速度のボタンがありません");
    return { host, button };
  }

  it("閉じているときは偽", () => {
    const { host } = create();
    expect(rateMenuOpen(host)).toBe(false);
  });

  it("押して開いたメニューを見分ける", () => {
    const { host } = create();
    const menuButton = player
      ?.getChild("ControlBar")
      ?.getChild("PlaybackRateMenuButton") as
      { pressButton(): void; unpressButton(): void } | undefined;
    if (menuButton === undefined) throw new Error("再生速度のボタンがありません");
    menuButton.pressButton();
    expect(rateMenuOpen(host)).toBe(true);
    menuButton.unpressButton();
    expect(rateMenuOpen(host)).toBe(false);
  });

  it("ポイントして開いたメニューを見分ける", () => {
    const { host, button } = create();
    const trigger = button.querySelector("button");
    if (trigger === null) throw new Error("再生速度のボタンがありません");
    trigger.dispatchEvent(new MouseEvent("mouseenter"));
    expect(rateMenuOpen(host)).toBe(true);
    button.dispatchEvent(new MouseEvent("mouseleave"));
    expect(rateMenuOpen(host)).toBe(false);
  });
});
