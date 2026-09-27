import { setDecoration } from "./intl";
import { englishMessages, mapMessages, replaceMessages } from "./messages";

// 疑似ロケールと、描画した画面の文言がすべてカタログ由来か利用者のデータであることを
// 確かめる helper である。テストでだけ使う（specs/023-english-i18n/research.md R-3）。
//
// 疑似ロケールはカタログのすべての文と書式関数の出力を ⟦ ⟧ で囲む。`string` の変数を
// 経由してカタログを通らずに描かれた英語は、画面で印を持たないので見つかる。

export const PSEUDO_OPEN = "⟦";
export const PSEUDO_CLOSE = "⟧";

function mark(text: string): string {
  return `${PSEUDO_OPEN}${text}${PSEUDO_CLOSE}`;
}

/**
 * enablePseudoLocale はカタログと書式関数を疑似ロケールに差し替える。戻すのは
 * resetLocale（vitest.setup.ts が各テストのあとに呼ぶ）。
 */
export function enablePseudoLocale(): void {
  replaceMessages(mapMessages(englishMessages, mark));
  setDecoration(mark);
}

/** resetLocale はカタログと書式関数を英語に戻す。 */
export function resetLocale(): void {
  replaceMessages(englishMessages);
  setDecoration(null);
}

const markedSpan = new RegExp(
  `${PSEUDO_OPEN}[^${PSEUDO_OPEN}${PSEUDO_CLOSE}]*${PSEUDO_CLOSE}`,
  "g",
);
// 文字と数字の無い残り（空白、句読点、記号）は文言として数えない。
const onlyPunctuation = /^[\s\p{P}\p{S}]*$/u;

/** checkedAttributes は読み上げ名と補足の文言を運ぶ属性である。 */
export const checkedAttributes = ["aria-label", "title", "placeholder", "alt"] as const;

/**
 * residue は、印の付いた範囲と利用者のデータを除いたあとの残りを返す。印は入れ子に
 * なり得る（カタログの文に別の文を埋め込む）ので、内側から外す。
 */
function residue(text: string, userData: readonly string[]): string {
  let rest = text;
  for (;;) {
    const next = rest.replace(markedSpan, " ");
    if (next === rest) break;
    rest = next;
  }
  for (const data of userData) {
    if (data !== "") rest = rest.split(data).join(" ");
  }
  return rest;
}

/** UncataloguedText はカタログを通らずに描かれた文言 1 つである。 */
export interface UncataloguedText {
  /** 見えるテキストなら "text"、属性ならその名前。 */
  where: "text" | (typeof checkedAttributes)[number];
  text: string;
}

/**
 * findUncataloguedText は、root の下の見えるテキストと `aria-label`・`title`・
 * `placeholder`・`alt` のうち、印を持たず、テストが与えた利用者のデータでもない文言を
 * 返す。疑似ロケールを有効にしてから描画した画面に使う。
 */
export function findUncataloguedText(
  root: Node,
  userData: readonly string[] = [],
): UncataloguedText[] {
  // 長い利用者のデータから外す（短いものが長いものの一部を先に消さないため）。
  const data = [...userData].sort((a, b) => b.length - a.length);
  const found: UncataloguedText[] = [];
  const document = root.ownerDocument ?? (root as Document);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
    const parent = node.parentElement;
    if (parent !== null && (parent.tagName === "SCRIPT" || parent.tagName === "STYLE")) {
      continue;
    }
    const text = node.textContent ?? "";
    if (!onlyPunctuation.test(residue(text, data))) found.push({ where: "text", text });
  }
  const elements =
    root instanceof Element
      ? [root, ...Array.from(root.querySelectorAll("*"))]
      : Array.from((root as ParentNode).querySelectorAll("*"));
  for (const element of elements) {
    for (const name of checkedAttributes) {
      const value = element.getAttribute(name);
      if (value === null) continue;
      if (!onlyPunctuation.test(residue(value, data)))
        found.push({ where: name, text: value });
    }
  }
  return found;
}

/**
 * expectCatalogTextOnly は findUncataloguedText が何か見つけたら、その一覧を添えて
 * 失敗する。
 */
export function expectCatalogTextOnly(
  root: Node,
  userData: readonly string[] = [],
): void {
  const found = findUncataloguedText(root, userData);
  if (found.length === 0) return;
  const lines = found.map((item) => `  ${item.where}: ${JSON.stringify(item.text)}`);
  throw new Error(`text rendered outside the i18n catalog:\n${lines.join("\n")}`);
}
