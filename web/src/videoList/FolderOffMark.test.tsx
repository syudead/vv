import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import type { PeerState } from "../api/peer";
import { remoteIdOffset } from "../lib/remoteId";
import { StaticPeer } from "../shell/PeerProvider";
import { FolderOffMark } from "./FolderOffMark";

describe("FolderOffMark", () => {
  afterEach(() => cleanup());

  const peer: PeerState = {
    links: [
      { slot: 1, state: "connected", name: "studio-pc", address: "http://pc:8080" },
      { slot: 2, state: "connected", name: "den-pc", address: "http://den:8080" },
    ],
  };

  it("スロット 2 の動画には 2 台目のリンクの名前を出す", () => {
    const video = { id: 2 * remoteIdOffset + 1, folderOff: true } as Video;
    render(
      <StaticPeer state={peer}>
        <FolderOffMark video={video} />
      </StaticPeer>,
    );
    expect(screen.getByText("Folder off on den-pc")).toBeDefined();
  });
});
