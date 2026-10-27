import path from "node:path";

// The Go servers the suite runs: the main node every spec uses, and a second node with its own data
// directory and port that the pairing spec starts and stops itself.
const runRoot = process.env.MDM_E2E_RUN_ROOT;
if (runRoot === undefined) throw new Error("MDM_E2E_RUN_ROOT is not configured");

export const serverBinary = path.join(
  runRoot,
  "bin",
  process.platform === "win32" ? "mdm.exe" : "mdm",
);

export interface Node {
  /** host:port the server listens on. */
  addr: string;
  dataDir: string;
}

export const mainNode: Node = {
  addr: "127.0.0.1:18080",
  dataDir: path.join(runRoot, "data"),
};

export const secondNode: Node = {
  addr: "127.0.0.1:18081",
  dataDir: path.join(runRoot, "second-node-data"),
};

export function nodeURL(node: Node): string {
  return `http://${node.addr}`;
}

/** The environment that starts a node's server. */
export function nodeEnv(node: Node): Record<string, string> {
  return { MDM_ADDR: node.addr, MDM_DATA_DIR: node.dataDir, MDM_LOG_LEVEL: "debug" };
}
