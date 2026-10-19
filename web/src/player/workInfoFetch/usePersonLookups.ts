import { useEffect, useRef, useState } from "react";

import { RequestFailed } from "../../api/client";
import type { PersonPluginItem } from "../../api/personPluginFetches";
import {
  runPersonLookup,
  type WorkInfoFetch,
  type WorkInfoField,
} from "../../api/workInfoPlugins";
import { type LookupState, lookupChangeFields, shownLookup } from "./lookupRules";

interface Session {
  result: WorkInfoFetch;
  controller: AbortController;
  /** Mirrors the rendered states so a result can be compared with the ones before it. */
  states: LookupState[];
  queue: number[];
  running: boolean;
}

export interface PersonLookupEvents {
  /** Role rows that the latest result made selectable for the first time. */
  onNewChanges: (fields: WorkInfoField[], result: WorkInfoFetch) => void;
  /** The fetch expired or was applied, so no lookup can run any more. */
  onStale: () => void;
}

function failedState(error: unknown): LookupState {
  return error instanceof RequestFailed && error.code === "plugin_changed"
    ? { status: "changed" }
    : { status: "failed", error };
}

/**
 * Request a fetch's planned lookups one at a time, in plan order, so each result shows as it
 * arrives; Try again joins the same queue because the plugin runtime runs one module at a time.
 */
export function usePersonLookups(videoId: number, events: PersonLookupEvents) {
  const [states, setStates] = useState<LookupState[]>([]);
  const session = useRef<Session | null>(null);
  const latest = useRef(events);
  latest.current = events;

  useEffect(() => () => session.current?.controller.abort(), []);

  const update = (current: Session, index: number, state: LookupState) => {
    current.states = current.states.map((item, at) => (at === index ? state : item));
    if (session.current === current) setStates(current.states);
  };

  const settle = async (current: Session, index: number) => {
    const { result, controller } = current;
    try {
      const answer = await runPersonLookup(
        videoId,
        result.fetchId,
        index,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      const before = lookupChangeFields(result, current.states);
      update(current, index, shownLookup(answer));
      const fresh = [...lookupChangeFields(result, current.states)].filter(
        (field) => !before.has(field),
      );
      if (fresh.length > 0) latest.current.onNewChanges(fresh, result);
    } catch (error) {
      if (controller.signal.aborted) return;
      update(current, index, failedState(error));
      if (error instanceof RequestFailed && error.code === "work_info_fetch_stale")
        latest.current.onStale();
    }
  };

  const drain = async (current: Session) => {
    if (current.running) return;
    current.running = true;
    for (let index = current.queue.shift(); index !== undefined;) {
      await settle(current, index);
      if (current.controller.signal.aborted) return;
      index = current.queue.shift();
    }
    current.running = false;
  };

  const stop = () => {
    session.current?.controller.abort();
    session.current = null;
  };

  return {
    states,
    start: (result: WorkInfoFetch) => {
      stop();
      const lookups = result.personLookups.map((): LookupState => ({
        status: "looking",
      }));
      const current: Session = {
        result,
        controller: new AbortController(),
        states: lookups,
        queue: lookups.map((_, index) => index),
        running: false,
      };
      session.current = current;
      setStates(lookups);
      void drain(current);
    },
    stop,
    retry: (index: number) => {
      const current = session.current;
      if (current === null || current.states[index]?.status !== "failed") return;
      update(current, index, { status: "looking" });
      current.queue.push(index);
      void drain(current);
    },
    toggleItem: (index: number, item: PersonPluginItem, on: boolean) => {
      const current = session.current;
      const state = current?.states[index];
      if (current === null || state?.status !== "shown") return;
      const items = on
        ? [...new Set([...state.items, item])]
        : state.items.filter((value) => value !== item);
      update(current, index, { ...state, items });
    },
  };
}

export type PersonLookups = ReturnType<typeof usePersonLookups>;
