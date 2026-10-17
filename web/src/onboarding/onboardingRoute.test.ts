import { describe, expect, it } from "vitest";

import { nextOnboardingStep, toOnboardingStep } from "../api/onboarding";
import { onboardingRedirect } from "./onboardingRoute";

// オンボードへ送る規則（docs/design-docs/home.md）と段階の順（R-3）。

describe("onboardingRedirect", () => {
  it("段階が done でない所有者は、どの経路からも /welcome へ送る", () => {
    for (const step of ["folders", "scan", "interests"] as const) {
      expect(onboardingRedirect(true, step, "/library")).toBe("/welcome");
      expect(onboardingRedirect(true, step, "/settings")).toBe("/welcome");
      expect(onboardingRedirect(true, step, "/")).toBe("/welcome");
      expect(onboardingRedirect(true, step, "/welcome")).toBeNull();
    }
  });

  it("段階が done の所有者の /welcome はホームへ送り、ほかの経路は送らない", () => {
    expect(onboardingRedirect(true, "done", "/welcome")).toBe("/");
    expect(onboardingRedirect(true, "done", "/library")).toBeNull();
  });

  it("閲覧者は送らない", () => {
    expect(onboardingRedirect(false, "folders", "/library")).toBeNull();
    expect(onboardingRedirect(false, "done", "/")).toBeNull();
  });
});

describe("段階の順", () => {
  it("手順を folders、scan、interests、done の順に進める", () => {
    expect(nextOnboardingStep("folders")).toBe("scan");
    expect(nextOnboardingStep("scan")).toBe("interests");
    expect(nextOnboardingStep("interests")).toBe("done");
    expect(nextOnboardingStep("done")).toBe("done");
  });

  it("知らない段階と欠けた段階は done と扱い、画面を止めない", () => {
    expect(toOnboardingStep("scan")).toBe("scan");
    expect(toOnboardingStep(undefined)).toBe("done");
    expect(toOnboardingStep("later")).toBe("done");
  });
});
