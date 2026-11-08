import { describe, expect, it, vi } from "vitest";

import { nearWatcher } from "./useNearViewport";

class FakeObserver {
  static last: FakeObserver;
  readonly observed = new Set<Element>();
  readonly callback: IntersectionObserverCallback;
  readonly options?: IntersectionObserverInit;
  constructor(
    callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit,
  ) {
    this.callback = callback;
    this.options = options;
    FakeObserver.last = this;
  }
  observe(element: Element) {
    this.observed.add(element);
  }
  unobserve(element: Element) {
    this.observed.delete(element);
  }
  report(target: Element, isIntersecting: boolean) {
    this.callback(
      [{ target, isIntersecting } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}

describe("nearWatcher", () => {
  it("calls an element's callback once when it comes within a viewport, then stops watching it", () => {
    const watch = nearWatcher(FakeObserver as unknown as typeof IntersectionObserver);
    const observer = FakeObserver.last;
    const far = document.createElement("img");
    const near = document.createElement("img");
    const onFar = vi.fn();
    const onNear = vi.fn();
    watch(far, onFar);
    watch(near, onNear);

    expect(observer.options?.rootMargin).toBe("100% 0px");
    observer.report(near, false);
    expect(onNear).not.toHaveBeenCalled();
    observer.report(near, true);
    observer.report(near, true);
    expect(onNear).toHaveBeenCalledTimes(1);
    expect([...observer.observed]).toEqual([far]);
    expect(onFar).not.toHaveBeenCalled();
  });

  it("stops watching an element whose image unmounts before it comes near", () => {
    const watch = nearWatcher(FakeObserver as unknown as typeof IntersectionObserver);
    const observer = FakeObserver.last;
    const element = document.createElement("img");
    const callback = vi.fn();
    const stop = watch(element, callback);
    stop();
    observer.report(element, true);
    expect(callback).not.toHaveBeenCalled();
    expect(observer.observed.size).toBe(0);
  });
});
