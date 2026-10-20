import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import {
  people,
  person,
  renderPage,
  sent,
  sentBody,
  server,
  installPersonServer,
} from "../../testing/personPage";

installPersonServer();

beforeEach(() => {
  server.plugins = [people];
});

const stored = { externalIds: [{ source: "fanza", value: "a1" }] };

async function openDialog(user: ReturnType<typeof userEvent.setup>) {
  renderPage();
  await user.click(await screen.findByRole("button", { name: "Fetch info…" }));
  return screen.findByRole("dialog", { name: "Fetch info for Aoi" });
}

async function searchByName(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await openDialog(user);
  await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
  await within(dialog).findByRole("textbox", { name: "Name" });
  await user.click(within(dialog).getByRole("button", { name: "Search" }));
  return dialog;
}

describe("PersonPage fetch info", () => {
  it("compares the result of the stored ID with no name search", async () => {
    server.person = person(stored);
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    const user = userEvent.setup();
    const dialog = await openDialog(user);

    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    expect(await within(dialog).findByRole("checkbox", { name: "Cup" })).toBeTruthy();
    expect(within(dialog).queryByRole("textbox", { name: "Name" })).toBeNull();
    expect(sent("POST", "/api/work-info-plugins/5/person-search")).toHaveLength(0);
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches")[0])).toEqual({
      pluginId: 5,
    });
  });

  it("searches by the person's name and fetches the chosen candidate's ID", async () => {
    server.candidates = [
      { source: "fanza", id: "x9", name: "Aoi Sora", url: "https://example.com/x9" },
    ];
    server.fetched = {
      name: "Aoi Sora",
      aliases: [],
      externalIds: [],
      profile: { cup: "C" },
    };
    const user = userEvent.setup();
    const dialog = await searchByName(user);

    await user.click(await within(dialog).findByRole("button", { name: /Aoi Sora/ }));

    expect(await within(dialog).findByRole("checkbox", { name: "Cup" })).toBeTruthy();
    expect(sentBody(sent("POST", "/api/work-info-plugins/5/person-search")[0])).toEqual({
      query: "Aoi",
    });
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches")[1])).toEqual({
      pluginId: 5,
      source: "fanza",
      id: "x9",
    });
  });

  it("shows no matches and applies nothing when the search finds no one", async () => {
    const user = userEvent.setup();
    const dialog = await searchByName(user);

    expect(await within(dialog).findByText("No matches. Try other words.")).toBeTruthy();
    expect(sent("POST", "/api/people/7/plugin-fetches")).toHaveLength(1);
    expect(server.sent.some((request) => request.url.pathname.endsWith("/apply"))).toBe(
      false,
    );
  });

  it("applies only the checked items, not the unchecked or kept ones", async () => {
    server.person = person({ ...stored, profile: { birthplace: "Tokyo" } });
    server.fetched = {
      name: "Aoi",
      aliases: [{ name: "Sora" }],
      externalIds: [{ source: "fanza", value: "a1" }],
      profile: { birthplace: "Osaka", heightCm: 160, bloodType: "A" },
    };
    server.applied = person({
      ...stored,
      profile: { birthplace: "Tokyo", bloodType: "A" },
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    await user.click(await within(dialog).findByRole("checkbox", { name: "Height" }));
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));

    expect(await screen.findByText("Applied to Aoi.")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches/f1/apply")[0])).toEqual({
      items: ["profile.bloodType", "aliases"],
    });
    expect(screen.getByText("Blood type")).toBeTruthy();
  });

  it("replaces a stored value only when its row is checked", async () => {
    server.person = person({ ...stored, profile: { birthplace: "Tokyo" } });
    server.fetched = {
      name: "Aoi",
      aliases: [],
      externalIds: [],
      profile: { birthplace: "Osaka", bloodType: "A" },
    };
    server.applied = person({
      ...stored,
      profile: { birthplace: "Osaka", bloodType: "A" },
    });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    const birthplace = await within(dialog).findByRole("checkbox", {
      name: /Birthplace/,
    });
    expect(birthplace.getAttribute("aria-checked")).toBe("false");
    await user.click(birthplace);
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));

    expect(await screen.findByText("Applied to Aoi.")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches/f1/apply")[0])).toEqual({
      items: ["profile.bloodType", "profile.birthplace"],
    });
  });

  it("keeps the stored face until the owner picks one of the fetched images", async () => {
    server.person = person({ ...stored, imageRevision: 2 });
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], imageCount: 2 };
    server.applied = person({ ...stored, imageRevision: 3 });
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    const keep = await within(dialog).findByRole("radio", { name: "Keep current" });
    expect(keep.getAttribute("aria-checked")).toBe("true");
    const apply = within(dialog).getByRole("button", { name: "Apply selected" });
    expect(apply).toHaveProperty("disabled", true);
    await user.click(within(dialog).getByRole("radio", { name: "Image 2" }));
    await user.click(apply);

    expect(await screen.findByText("Applied to Aoi.")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches/f1/apply")[0])).toEqual({
      items: ["image"],
      imageIndex: 1,
    });
  });

  it("searches by name for a person with a stored ID, without fetching first", async () => {
    server.person = person(stored);
    const user = userEvent.setup();
    const dialog = await openDialog(user);

    await user.click(within(dialog).getByRole("button", { name: "Search by name" }));
    await user.click(within(dialog).getByRole("button", { name: "Search" }));

    expect(await within(dialog).findByText("No matches. Try other words.")).toBeTruthy();
    expect(within(dialog).queryByText(/has no ID for this person/)).toBeNull();
    expect(sent("POST", "/api/people/7/plugin-fetches")).toHaveLength(0);
    expect(sentBody(sent("POST", "/api/work-info-plugins/5/person-search")[0])).toEqual({
      query: "Aoi",
    });
  });

  it("lists the values the apply left out", async () => {
    server.person = person(stored);
    server.fetched = {
      name: "Aoi",
      aliases: [],
      externalIds: [
        { source: "fanza", value: "a1" },
        { source: "fanza", value: "b2" },
      ],
    };
    server.skipped = [
      { item: "externalIds", value: "fanza: b2", reason: "taken", holderName: "Mio" },
    ];
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await user.click(
      await within(dialog).findByRole("button", { name: "Apply selected" }),
    );

    const line = await within(dialog).findByRole("listitem");
    expect(line.textContent).toBe("External IDs: fanza: b2 — already linked to Mio");
    expect(within(dialog).getByRole("button", { name: "Done" })).toBeTruthy();
  });
});
