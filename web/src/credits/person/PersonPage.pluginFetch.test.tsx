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

  it("opens Fetch info when the page is opened with ?fetch=1", async () => {
    renderPage("/people/7?fetch=1");

    expect(
      await screen.findByRole("dialog", { name: "Fetch info for Aoi" }),
    ).toBeTruthy();
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
      items: ["profile.bloodType"],
      aliases: ["Sora"],
      externalIds: [],
      links: [],
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
      aliases: [],
      externalIds: [],
      links: [],
    });
  });

  it("selects every replacement at once with Select all", async () => {
    server.person = person({ ...stored, profile: { birthplace: "Tokyo", cup: "C" } });
    server.fetched = {
      name: "Aoi",
      aliases: [],
      externalIds: [],
      profile: { birthplace: "Osaka", cup: "D" },
    };
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    await user.click(await within(dialog).findByRole("button", { name: "Select all" }));
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));

    expect(await screen.findByText("Applied to Aoi.")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches/f1/apply")[0])).toEqual({
      items: ["profile.birthplace", "profile.cup"],
      aliases: [],
      externalIds: [],
      links: [],
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
      aliases: [],
      externalIds: [],
      links: [],
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

  it("adds only the aliases, IDs and links left pressed", async () => {
    server.person = person({ ...stored, profile: { links: ["https://a.test"] } });
    server.fetched = {
      name: "Aoi Sora",
      aliases: [{ name: "Sora" }],
      externalIds: [
        { source: "fanza", value: "a1" },
        { source: "fanza", value: "b2" },
      ],
      profile: { links: ["https://a.test", "https://b.test"] },
    };
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    // The fetched name is offered as an alias; values the person has are not choices.
    const fetchedName = await within(dialog).findByRole("button", { name: "Aoi Sora" });
    expect(fetchedName.getAttribute("aria-pressed")).toBe("true");
    expect(within(dialog).queryByRole("button", { name: "fanza a1" })).toBeNull();
    await user.click(fetchedName);
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));

    expect(await screen.findByText("Applied to Aoi.")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches/f1/apply")[0])).toEqual({
      items: [],
      aliases: ["Sora"],
      externalIds: [{ source: "fanza", value: "b2" }],
      links: ["https://b.test"],
    });
  });

  it("goes back from a comparison to search by another of the person's names", async () => {
    server.person = person({ ...stored, aliases: [{ name: "Sora" }] });
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await within(dialog).findByRole("checkbox", { name: "Cup" });

    await user.click(within(dialog).getByRole("button", { name: "Back" }));
    await user.click(within(dialog).getByRole("button", { name: "Search by name" }));
    await user.click(within(dialog).getByRole("button", { name: "Sora" }));

    expect(await within(dialog).findByText("No matches. Try other words.")).toBeTruthy();
    expect(within(dialog).getByRole("textbox", { name: "Name" })).toHaveProperty(
      "value",
      "Sora",
    );
    expect(sentBody(sent("POST", "/api/work-info-plugins/5/person-search")[0])).toEqual({
      query: "Sora",
    });
  });

  it("fetches the one handled link with only the plugin and names it on the source line", async () => {
    server.handledLinks = ["https://example.com/p/1"];
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    const user = userEvent.setup();
    const dialog = await openDialog(user);

    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    expect(await within(dialog).findByRole("checkbox", { name: "Cup" })).toBeTruthy();
    expect(
      within(dialog).getByText("From people · https://example.com/p/1"),
    ).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches")[0])).toEqual({
      pluginId: 5,
    });
  });

  it("offers the handled links as a choice and fetches the pressed one", async () => {
    server.handledLinks = ["https://example.com/p/1", "https://example.com/p/2?tab=a"];
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    const list = await within(dialog).findByRole("region", {
      name: "Links people handles",
    });
    expect(within(dialog).getByRole("button", { name: "Fetch" })).toHaveProperty(
      "disabled",
      true,
    );
    await user.click(within(list).getByRole("button", { name: /\/p\/2\?tab=a/ }));

    expect(
      await within(dialog).findByText("From people · https://example.com/p/2?tab=a"),
    ).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches")[1])).toEqual({
      pluginId: 5,
      link: "https://example.com/p/2?tab=a",
    });
    // Back keeps the choice, so another link is one press away.
    await user.click(within(dialog).getByRole("button", { name: "Back" }));
    expect(
      within(dialog).getByRole("region", { name: "Links people handles" }),
    ).toBeTruthy();
  });

  it("resends the picked link when the result went out of date", async () => {
    server.handledLinks = ["https://example.com/p/1", "https://example.com/p/2"];
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    server.staleApply = true;
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await user.click(await within(dialog).findByRole("button", { name: /\/p\/2/ }));
    await user.click(
      await within(dialog).findByRole("button", { name: "Apply selected" }),
    );

    await user.click(await within(dialog).findByRole("button", { name: "Fetch again" }));

    await within(dialog).findByRole("button", { name: "Apply selected" });
    const fetches = sent("POST", "/api/people/7/plugin-fetches");
    expect(fetches).toHaveLength(3);
    expect(sentBody(fetches[2])).toEqual({
      pluginId: 5,
      link: "https://example.com/p/2",
    });
  });

  it("fetches a link typed into Name instead of searching it", async () => {
    server.handledLinks = ["https://example.com/p/1"];
    server.fetched = { name: "Aoi", aliases: [], externalIds: [], profile: { cup: "C" } };
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Search by name" }));
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    await user.clear(field);
    await user.type(field, "https://example.com/p/1");

    expect(
      within(dialog).getByText("Fetches the person on this page instead of searching."),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    expect(await within(dialog).findByRole("checkbox", { name: "Cup" })).toBeTruthy();
    expect(sent("POST", "/api/work-info-plugins/5/person-search")).toHaveLength(0);
    expect(sentBody(sent("POST", "/api/people/7/plugin-fetches")[0])).toEqual({
      pluginId: 5,
      link: "https://example.com/p/1",
    });
    await user.click(within(dialog).getByRole("button", { name: "Back to the matches" }));
    expect(within(dialog).getByRole("textbox", { name: "Name" })).toHaveProperty(
      "value",
      "https://example.com/p/1",
    );
  });

  it("keeps a typed link the plugin does not handle and says why, searching nothing", async () => {
    server.plugins = [{ ...people, domains: ["example.com", "other.example"] }];
    const user = userEvent.setup();
    const dialog = await openDialog(user);
    await user.click(within(dialog).getByRole("button", { name: "Search by name" }));
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    await user.clear(field);
    await user.type(field, "https://elsewhere.test/a");
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));

    expect(
      await within(dialog).findByText(
        "people doesn't handle links on elsewhere.test. It accepts links from example.com, other.example.",
      ),
    ).toBeTruthy();
    expect(field).toHaveProperty("value", "https://elsewhere.test/a");
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(sent("POST", "/api/work-info-plugins/5/person-search")).toHaveLength(0);

    await user.type(field, "x");
    expect(within(dialog).queryByText(/doesn't handle links/)).toBeNull();
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
