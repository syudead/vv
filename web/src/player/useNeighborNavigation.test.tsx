import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useNeighborNavigation } from "./useNeighborNavigation";

const navigate = vi.fn();

vi.mock("react-router", () => ({ useNavigate: () => navigate }));
vi.mock("./usePreviousVideo", () => ({ usePreviousVideo: () => undefined }));

describe("useNeighborNavigation", () => {
  it("関連動画を読む前は前後が無く、playNext は移らない", () => {
    const { result } = renderHook(() =>
      useNeighborNavigation({
        related: { kind: "loading", id: 12 },
        trail: [],
        linkState: { from: "/", trail: [] },
        backTo: "/",
        videoTo: (id) => `/videos/${String(id)}`,
        keepPlaying: false,
      }),
    );
    expect(result.current.previous).toBeUndefined();
    expect(result.current.next).toBeUndefined();
    result.current.playNext();
    expect(navigate).not.toHaveBeenCalled();
  });
});
