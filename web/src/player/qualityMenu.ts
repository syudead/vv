import videojs from "video.js";

import type { TranscodeQuality } from "../api/client";
import { t } from "../i18n";
import type { PlaybackQuality } from "../preferences/playbackQuality";

/** Reuse video.js menu behavior; emit quality choices and let VideoPlayer replace the source. */

export const qualityMenuName = "QualityMenuButton";

/** Selection events carry {@link QualitySelection} as the second argument. */
export const qualitySelectEvent = "vvqualityselect";

export interface QualitySelection {
  quality: PlaybackQuality;
}

export interface QualityMenuState {
  options: readonly TranscodeQuality[];
  current: PlaybackQuality;
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

/** Store state externally because the parent constructor calls createItems before subclass fields exist. */
const states = new WeakMap<object, QualityMenuState>();

function stateOf(component: object): QualityMenuState {
  return states.get(component) ?? emptyState;
}

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
    // Render unavailable-quality guidance outside the component list so arrow keys skip it.
    if (stateOf(this).options.length === 0) {
      const note = document.createElement("li");
      note.className = "vjs-menu-title vv-quality-note";
      note.textContent = t.player.controls.qualityNoSmaller;
      menu.contentEl().appendChild(note);
    }
    return menu;
  }

  /** Update labels and selection without rebuilding an open menu when options are unchanged; avoid video.js's existing setState name. */
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

export function setQualityMenu(player: Player, state: QualityMenuState): void {
  const bar = (player as unknown as { getChild?(name: string): unknown }).getChild?.(
    "ControlBar",
  ) as { getChild(name: string): unknown } | undefined;
  const menu = bar?.getChild(qualityMenuName) as QualityMenuButton | undefined;
  menu?.setQualityState(state);
}
