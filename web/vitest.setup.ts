// Use Vitest assertions without adding jest-dom dependencies.
import { cleanup, configure } from "@testing-library/react";
import { afterEach, beforeEach, vi } from "vitest";

import { resetLocale } from "./src/i18n/pseudo";

// Allow slower initial renders under concurrent CI coverage, but stay below the five-second test timeout so failures identify the awaited condition.
configure({ asyncUtilTimeout: 3000 });

afterEach(() => {
  cleanup();

  resetLocale();

  // Clear shared jsdom storage to prevent view preferences leaking across tests; Node-only tests have no localStorage.
  if (typeof localStorage !== "undefined" && typeof localStorage.clear === "function") {
    localStorage.clear();
  }
});

// Testing Library detects fake timers through jest; expose only Vitest timer advancement so user-event waits can complete.
Object.assign(globalThis, {
  jest: { advanceTimersByTime: vi.advanceTimersByTime.bind(vi) },
});

// Radix Slider requires ResizeObserver, which jsdom lacks.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// cmdk calls scrollIntoView, which jsdom lacks.
if (
  typeof Element !== "undefined" &&
  typeof Element.prototype.scrollIntoView !== "function"
) {
  Element.prototype.scrollIntoView = function scrollIntoView() {};
}

// Remove Sonner visual styles before each test: jsdom does not render them, but matching their rules slows every role query.
const sonnerStylePrefix = "[data-sonner-toaster]";
beforeEach(() => {
  if (typeof document === "undefined") return;
  for (const style of document.head.querySelectorAll("style")) {
    if (style.textContent?.startsWith(sonnerStylePrefix)) style.remove();
  }
});
