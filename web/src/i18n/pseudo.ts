import { setDecoration } from "./intl";
import { englishMessages, mapMessages, replaceMessages } from "./messages";

// Test-only helpers verify rendered copy comes from the catalog or supplied user data.
//
// Wrap catalog and formatter output in pseudo-locale markers to expose unmarked literal strings.

export const PSEUDO_OPEN = "⟦";
export const PSEUDO_CLOSE = "⟧";

function mark(text: string): string {
  return `${PSEUDO_OPEN}${text}${PSEUDO_CLOSE}`;
}

/** Restore with resetLocale, called after each test by vitest.setup.ts. */
export function enablePseudoLocale(): void {
  replaceMessages(mapMessages(englishMessages, mark));
  setDecoration(mark);
}

export function resetLocale(): void {
  replaceMessages(englishMessages);
  setDecoration(null);
}

const markedSpan = new RegExp(
  `${PSEUDO_OPEN}[^${PSEUDO_OPEN}${PSEUDO_CLOSE}]*${PSEUDO_CLOSE}`,
  "g",
);
// Ignore residue containing no letters or digits.
const onlyPunctuation = /^[\s\p{P}\p{S}]*$/u;

export const checkedAttributes = ["aria-label", "title", "placeholder", "alt"] as const;

/** Strip nested decoration from the inside out before removing user data. */
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

export interface UncataloguedText {
  /** text for visible content; otherwise the attribute name. */
  where: "text" | (typeof checkedAttributes)[number];
  text: string;
}

/**
 * Run after enabling the pseudo-locale; inspect visible text and accessible-text attributes,
 * excluding supplied user data.
 */
export function findUncataloguedText(
  root: Node,
  userData: readonly string[] = [],
): UncataloguedText[] {
  // Remove longer user strings first so shorter substrings cannot corrupt them.
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

export function expectCatalogTextOnly(
  root: Node,
  userData: readonly string[] = [],
): void {
  const found = findUncataloguedText(root, userData);
  if (found.length === 0) return;
  const lines = found.map((item) => `  ${item.where}: ${JSON.stringify(item.text)}`);
  throw new Error(`text rendered outside the i18n catalog:\n${lines.join("\n")}`);
}
