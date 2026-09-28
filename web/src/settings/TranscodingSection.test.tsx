import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { EncoderAvailability, TranscodingSettings } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { HARDWARE_ENCODING_GUIDE_URL } from "./docsLinks";
import TranscodingSection from "./TranscodingSection";

const URL = "/api/settings/transcoding";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function encoders(
  overrides: Partial<
    Record<EncoderAvailability["encoder"], Partial<EncoderAvailability>>
  > = {},
): EncoderAvailability[] {
  return (["nvenc", "qsv", "vaapi", "videotoolbox"] as const).map((encoder) => ({
    encoder,
    state: "unavailable",
    reason: encoder === "videotoolbox" ? "unsupported_os" : "encoder_missing",
    ...overrides[encoder],
  }));
}

function settings(values: Partial<TranscodingSettings> = {}): TranscodingSettings {
  return {
    videoEncoder: "software",
    effectiveEncoder: "software",
    checking: false,
    encoders: encoders(),
    ...values,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function radio(name: string | RegExp): HTMLInputElement {
  return screen.getByRole("radio", { name }) as HTMLInputElement;
}

function inUse(): string {
  return screen.getByText("In use now").nextElementSibling?.textContent ?? "";
}

describe("TranscodingSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => vi.unstubAllGlobals());

  it("selects software when nothing has been chosen", async () => {
    fetchMock.mockResolvedValue(json(settings()));
    render(<TranscodingSection />);

    expect(
      await screen.findByRole("heading", { level: 2, name: "Video conversion" }),
    ).toBeDefined();
    await screen.findByRole("radiogroup", { name: "Video encoder" });
    expect(radio("Software").checked).toBe(true);
    expect(radio("Software").disabled).toBe(false);
    expect(radio("Automatic").disabled).toBe(false);
    expect(inUse()).toBe("Software");
    expect(fetchMock).toHaveBeenCalledWith(URL, expect.anything());
  });

  it("disables unavailable encoders and states why", async () => {
    fetchMock.mockResolvedValue(
      json(
        settings({
          encoders: encoders({
            vaapi: { state: "available", reason: undefined },
            nvenc: { reason: "check_failed" },
            qsv: { reason: "timed_out" },
          }),
        }),
      ),
    );
    render(<TranscodingSection />);
    await screen.findByRole("radiogroup");

    const nvenc = radio("NVENC (NVIDIA)");
    expect(nvenc.disabled).toBe(true);
    expect(nvenc.getAttribute("aria-describedby")).not.toBeNull();
    expect(
      document.getElementById(nvenc.getAttribute("aria-describedby") ?? "")?.textContent,
    ).toBe("The test encode failed");
    expect(radio("Quick Sync (Intel)").disabled).toBe(true);
    expect(screen.getByText("The test encode took too long")).toBeDefined();
    expect(radio("VideoToolbox (macOS)").disabled).toBe(true);
    expect(
      screen.getByText("Not supported on this server's operating system"),
    ).toBeDefined();
    expect(radio("VAAPI (Intel/AMD)").disabled).toBe(false);
  });

  it("saves on selection, locks the choices while saving and shows the new effective encoder", async () => {
    const put = deferred<Response>();
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === URL && init?.method === "PUT") return put.promise;
      return Promise.resolve(
        json(
          settings({
            encoders: encoders({ vaapi: { state: "available", reason: undefined } }),
          }),
        ),
      );
    });
    const user = userEvent.setup();
    render(<TranscodingSection />);
    await screen.findByRole("radiogroup");

    await user.click(radio("VAAPI (Intel/AMD)"));

    const putCall = fetchMock.mock.calls.find(([, init]) => init?.method === "PUT");
    expect(putCall?.[0]).toBe(URL);
    expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({ videoEncoder: "vaapi" });
    expect(screen.getByRole("status").textContent).toBe("Saving…");
    expect(radio("VAAPI (Intel/AMD)").checked).toBe(true);
    expect(radio("Software").disabled).toBe(true);
    expect(radio("Automatic").disabled).toBe(true);

    put.resolve(
      json(
        settings({
          videoEncoder: "vaapi",
          effectiveEncoder: "vaapi",
          encoders: encoders({ vaapi: { state: "available", reason: undefined } }),
        }),
      ),
    );
    await waitFor(() => expect(inUse()).toBe("VAAPI (Intel/AMD)"));
    expect(screen.queryByText("Saving…")).toBeNull();
    expect(radio("Software").disabled).toBe(false);
    expect(radio("VAAPI (Intel/AMD)").checked).toBe(true);
  });

  it("restores the previous choice and shows the reason when saving fails", async () => {
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === URL && init?.method === "PUT") {
        return Promise.resolve(
          json({ code: "internal", message: "internal error" }, 500),
        );
      }
      return Promise.resolve(json(settings()));
    });
    const user = userEvent.setup();
    render(<TranscodingSection />);
    await screen.findByRole("radiogroup");

    await user.click(radio("Automatic"));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't change the encoder: /);
    expect(screen.getByRole("region", { name: "Video conversion" }).contains(alert)).toBe(
      true,
    );
    expect(radio("Software").checked).toBe(true);
    expect(radio("Automatic").checked).toBe(false);
    expect(inUse()).toBe("Software");
  });

  it("refreshes availability when the server rejects an unavailable encoder", async () => {
    let gets = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === URL && init?.method === "PUT") {
        return Promise.resolve(
          json(
            { code: "conflict", message: "unavailable", reason: "encoder_unavailable" },
            409,
          ),
        );
      }
      gets += 1;
      return Promise.resolve(
        json(
          settings({
            encoders: encoders({
              nvenc:
                gets === 1
                  ? { state: "available", reason: undefined }
                  : { state: "unavailable", reason: "check_failed" },
            }),
          }),
        ),
      );
    });
    const user = userEvent.setup();
    render(<TranscodingSection />);
    await screen.findByRole("radiogroup");

    await user.click(radio("NVENC (NVIDIA)"));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "That video encoder isn't available on this server.",
    );
    await waitFor(() => expect(radio("NVENC (NVIDIA)").disabled).toBe(true));
    expect(radio("Software").checked).toBe(true);
  });

  it("warns above the choices when the selected encoder fell back to software", async () => {
    fetchMock.mockResolvedValue(
      json(
        settings({
          videoEncoder: "nvenc",
          effectiveEncoder: "software",
          fallbackReason: "selected_unavailable",
        }),
      ),
    );
    render(<TranscodingSection />);
    await screen.findByRole("radiogroup");

    const warning = screen.getByText(
      "The selected encoder isn't available on this server, so videos are converted with software.",
    );
    expect(warning.className).toContain("text-warning");
    const group = screen.getByRole("radiogroup");
    expect(
      warning.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(radio("NVENC (NVIDIA)").checked).toBe(true);
    expect(inUse()).toBe("Software");
  });

  it("links the description to the hardware encoding guide in a new tab", async () => {
    fetchMock.mockResolvedValue(json(settings()));
    render(<TranscodingSection />);

    const link = await screen.findByRole("link", {
      name: /How to set up hardware encoding/,
    });
    expect(HARDWARE_ENCODING_GUIDE_URL).toBe(
      "https://syudead.github.io/vv/docs/how-to/running-vv#hardware-encoding",
    );
    expect(link.getAttribute("href")).toBe(HARDWARE_ENCODING_GUIDE_URL);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noreferrer");
  });

  it("shows checking and polls until the checks finish", async () => {
    let gets = 0;
    fetchMock.mockImplementation(() => {
      gets += 1;
      return Promise.resolve(
        json(
          gets < 3
            ? settings({
                checking: true,
                encoders: encoders({
                  nvenc: { state: "checking", reason: undefined },
                  vaapi: { state: "checking", reason: undefined },
                }),
              })
            : settings({
                encoders: encoders({ vaapi: { state: "available", reason: undefined } }),
              }),
        ),
      );
    });
    render(<TranscodingSection pollIntervalMs={5} />);

    expect(
      (
        await screen.findByText("Checking which encoders this server can use…")
      ).getAttribute("role"),
    ).toBe("status");
    expect(radio("VAAPI (Intel/AMD)").disabled).toBe(true);
    expect(screen.getAllByText("Checking…")).toHaveLength(2);

    await waitFor(() =>
      expect(
        screen.queryByText("Checking which encoders this server can use…"),
      ).toBeNull(),
    );
    expect(radio("VAAPI (Intel/AMD)").disabled).toBe(false);
    expect(screen.getByText("Available")).toBeDefined();
    const callsAfterDone = fetchMock.mock.calls.length;
    expect(callsAfterDone).toBe(3);
    await new Promise((done) => setTimeout(done, 30));
    expect(fetchMock.mock.calls.length).toBe(callsAfterDone);
  });

  it("shows a retryable error when the settings can't be loaded", async () => {
    fetchMock.mockResolvedValueOnce(json({ code: "internal", message: "x" }, 500));
    fetchMock.mockResolvedValueOnce(json(settings()));
    const user = userEvent.setup();
    render(<TranscodingSection />);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load the video conversion settings: /);
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("radiogroup")).toBeDefined();
  });

  it("renders only catalog text", async () => {
    enablePseudoLocale();
    fetchMock.mockImplementation((_input, init) => {
      if (init?.method === "PUT") {
        return Promise.resolve(json({ code: "internal", message: "x" }, 500));
      }
      return Promise.resolve(
        json(
          settings({
            videoEncoder: "qsv",
            fallbackReason: "selected_unavailable",
            checking: true,
            encoders: encoders({
              nvenc: { state: "checking", reason: undefined },
              vaapi: { state: "available", reason: undefined },
              qsv: { reason: "check_failed" },
            }),
          }),
        ),
      );
    });
    const user = userEvent.setup();
    const { container } = render(<TranscodingSection pollIntervalMs={60_000} />);
    const group = await screen.findByRole("radiogroup");
    expectCatalogTextOnly(container);

    const auto = within(group).getAllByRole("radio").at(-1);
    if (auto === undefined) throw new Error("no radio");
    await user.click(auto);
    await screen.findByRole("alert");
    expectCatalogTextOnly(container);
  });
});
