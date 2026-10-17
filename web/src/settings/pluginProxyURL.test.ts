import { describe, expect, it } from "vitest";

import { isPluginProxyURL } from "./pluginProxyURL";

describe("isPluginProxyURL", () => {
  it.each([
    "",
    "http://proxy.example",
    "https://proxy.example:3128",
    "socks5://user:pass@proxy.example:1080",
    "http://192.168.1.2:8080",
    "http://[::1]:3128",
    // 2048 文字ちょうどで、UTF-8 では 2048 バイトを超える。
    `http://${"ü".repeat(2048 - "http://".length)}`,
  ])("accepts %j", (value) => {
    expect(isPluginProxyURL(value)).toBe(true);
  });

  it.each([
    "proxy.example:3128",
    "ftp://proxy.example",
    "http://",
    "http://proxy.example/",
    "http://proxy.example/path",
    "http://proxy.example?x=1",
    "http://proxy.example#top",
    "http://proxy.example:",
    "http://proxy.example:0",
    "http://proxy.example:65536",
    "http://user@:3128",
    "http://proxy example",
    `http://${"a".repeat(2048)}`,
    `http://${"ü".repeat(2048 - "http://".length + 1)}`,
  ])("refuses %j", (value) => {
    expect(isPluginProxyURL(value)).toBe(false);
  });
});
