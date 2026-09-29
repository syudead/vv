import videojs from "video.js";

import type { TranscodeQuality } from "../api/client";
import { t } from "../i18n";
import type { PlaybackQuality } from "../preferences/playbackQuality";

/**
 * 操作バーの画質メニューである（specs/027-playback-quality/research.md R-4、ui-design.md
 * 「Control bar: quality menu」）。
 *
 * 再生速度と同じ video.js の `MenuButton` の部品にして、箱・余白・開き方・キーボード・
 * Esc の扱いを再生速度とそろえる。部品は選ばれた画質を知らせるだけで、source の差し替えは
 * VideoPlayer.tsx が行う（R-5）。選択肢と今の画質は VideoPlayer.tsx が setQualityMenu で渡す。
 */

/** qualityMenuName は video.js に登録する部品の名前である（`controlBarChildren` では qualityMenuButton）。 */
export const qualityMenuName = "QualityMenuButton";

/** qualitySelectEvent は項目が選ばれたときにプレイヤーへ出すイベントで、第 2 引数に {@link QualitySelection} を渡す。 */
export const qualitySelectEvent = "vvqualityselect";

export interface QualitySelection {
  quality: PlaybackQuality;
}

export interface QualityMenuState {
  /** 選べる縮めた画質（quality.ts の qualityOptions。大きい順）。 */
  options: readonly TranscodeQuality[];
  /** 今再生している画質。 */
  current: PlaybackQuality;
  /** 元の画質の表示の短辺（`1080p`）。寸法の無い動画は undefined。 */
  sourceSize: string | undefined;
}

const emptyState: QualityMenuState = {
  options: [],
  current: "original",
  sourceSize: undefined,
};

type Player = ReturnType<typeof videojs>;

interface ComponentLike {
  el(): HTMLElement;
  player(): Player;
}

interface MenuLike extends ComponentLike {
  contentEl(): HTMLElement;
}

interface MenuItemLike extends ComponentLike {
  handleClick(event?: Event): void;
  selected(value: boolean): void;
}

interface MenuButtonLike extends ComponentLike {
  items?: MenuItemLike[];
  menu?: MenuLike;
  update(): void;
  controlText(text: string): void;
  createEl(): HTMLElement;
  createMenu(): MenuLike;
  buildCSSClass(): string;
  buildWrapperCSSClass(): string;
}

type MenuItemClass = new (
  player: Player,
  options: Record<string, unknown>,
) => MenuItemLike;
type MenuButtonClass = new (
  player: Player,
  options?: Record<string, unknown>,
) => MenuButtonLike;

const MenuItem = videojs.getComponent("MenuItem") as unknown as MenuItemClass;
const MenuButton = videojs.getComponent("MenuButton") as unknown as MenuButtonClass;

/**
 * 部品の状態。video.js の MenuButton は親の constructor の中で createItems を呼ぶので、
 * 子の class のフィールドはまだ無い。状態は部品ごとにここへ置く。
 */
const states = new WeakMap<object, QualityMenuState>();

function stateOf(component: object): QualityMenuState {
  return states.get(component) ?? emptyState;
}

/** buttonLabel はボタンに出す今の画質の短い文字である（ui-design.md「Button label」）。 */
export function buttonLabel(state: QualityMenuState): string {
  const c = t.player.controls;
  if (state.current !== "original") return c.qualityName(state.current);
  return state.sourceSize === undefined
    ? c.qualityOriginalShort
    : c.qualityName(state.sourceSize);
}

function originalLabel(state: QualityMenuState): string {
  const c = t.player.controls;
  return state.sourceSize === undefined
    ? c.qualityOriginal
    : c.qualityOriginalOf(state.sourceSize);
}

class QualityMenuItem extends MenuItem {
  quality: PlaybackQuality;

  constructor(
    player: Player,
    options: { label: string; quality: PlaybackQuality; selected: boolean },
  ) {
    super(player, { ...options, selectable: true, multiSelectable: false });
    this.quality = options.quality;
  }

  override handleClick(event?: Event) {
    super.handleClick(event);
    const selection: QualitySelection = { quality: this.quality };
    (this.player() as unknown as { trigger(event: string, hash: unknown): void }).trigger(
      qualitySelectEvent,
      selection,
    );
  }
}

class QualityMenuButton extends MenuButton {
  constructor(player: Player, options?: Record<string, unknown>) {
    super(player, options);
    this.controlText(t.player.controls.quality);
    this.renderLabel();
  }

  override createEl() {
    const el = super.createEl();
    const label = document.createElement("div");
    label.className = "vv-quality-value";
    label.setAttribute("aria-hidden", "true");
    el.appendChild(label);
    return el;
  }

  override buildCSSClass() {
    return `vv-quality ${super.buildCSSClass()}`;
  }

  override buildWrapperCSSClass() {
    return `vv-quality ${super.buildWrapperCSSClass()}`;
  }

  createItems(): MenuItemLike[] {
    const state = stateOf(this);
    const player = this.player();
    return [
      new QualityMenuItem(player, {
        label: originalLabel(state),
        quality: "original",
        selected: state.current === "original",
      }),
      ...state.options.map(
        (quality) =>
          new QualityMenuItem(player, {
            label: t.player.controls.qualityName(quality),
            quality,
            selected: state.current === quality,
          }),
      ),
    ];
  }

  override createMenu() {
    const menu = super.createMenu();
    // 選べる画質が無い動画は、押せない補足の行を「元の画質」の下に置く（Edge Case 1）。
    // 部品として足さないので、↑↓ のフォーカスはここに止まらない。
    if (stateOf(this).options.length === 0) {
      const note = document.createElement("li");
      note.className = "vjs-menu-title vv-quality-note";
      note.textContent = t.player.controls.qualityNoSmaller;
      menu.contentEl().appendChild(note);
    }
    return menu;
  }

  /**
   * setQualityState は選択肢と今の画質を反映する。選択肢が同じなら項目の印とボタンの文字だけを
   * 変え、開いているメニューを作り直さない（項目を押した処理の途中で呼ばれるため）。
   * video.js の部品はどれも setState を持つので、名前を分ける。
   */
  setQualityState(next: QualityMenuState) {
    const previous = stateOf(this);
    states.set(this, next);
    const sameItems =
      previous.sourceSize === next.sourceSize &&
      previous.options.length === next.options.length &&
      previous.options.every((quality, index) => next.options[index] === quality);
    if (sameItems && this.items !== undefined) {
      for (const item of this.items) {
        item.selected((item as QualityMenuItem).quality === next.current);
      }
    } else {
      this.update();
    }
    this.renderLabel();
  }

  renderLabel() {
    const label = this.el().querySelector(".vv-quality-value");
    if (label !== null) label.textContent = buttonLabel(stateOf(this));
  }
}

videojs.registerComponent(qualityMenuName, QualityMenuButton as never);

/** setQualityMenu は、プレイヤーの操作バーの画質メニューに選択肢と今の画質を渡す。 */
export function setQualityMenu(player: Player, state: QualityMenuState): void {
  const bar = (player as unknown as { getChild?(name: string): unknown }).getChild?.(
    "ControlBar",
  ) as { getChild(name: string): unknown } | undefined;
  const menu = bar?.getChild(qualityMenuName) as QualityMenuButton | undefined;
  menu?.setQualityState(state);
}
