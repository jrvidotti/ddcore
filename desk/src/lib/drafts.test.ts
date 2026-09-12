import { beforeEach, describe, expect, it } from "vitest";
import {
  DRAFT_PREFIX,
  DRAFT_TTL_MS,
  clearDraft,
  draftDecision,
  draftKey,
  pruneDrafts,
  readDraft,
  writeDraft,
  type Draft,
  type DraftStore,
} from "./drafts";

/** A Storage the tests can inspect — the real one is not there in node. */
class FakeStore implements DraftStore {
  map = new Map<string, string>();
  get length() { return this.map.size; }
  key(i: number) { return [...this.map.keys()][i] ?? null; }
  getItem(k: string) { return this.map.get(k) ?? null; }
  setItem(k: string, v: string) { this.map.set(k, v); }
  removeItem(k: string) { this.map.delete(k); }
}

const NOW = Date.UTC(2026, 0, 10);
const draft = (over: Partial<Draft> = {}): Draft =>
  ({ doctype: "Task", name: "TASK-1", doc: { name: "TASK-1", subject: "typed" }, savedAt: NOW, modified: "2026-01-01 10:00:00", ...over });

let store: FakeStore;
beforeEach(() => { store = new FakeStore(); });

describe("draftKey", () => {
  it("separates users, doctypes and records", () => {
    expect(draftKey("ann@example.com", "Task", "TASK-1")).toBe(`${DRAFT_PREFIX}ann@example.com:Task:TASK-1`);
    expect(draftKey("bob@example.com", "Task", "TASK-1")).not.toBe(draftKey("ann@example.com", "Task", "TASK-1"));
  });

  it("keys the unsaved record under 'new'", () => {
    expect(draftKey("ann@example.com", "Task", "new")).toBe(`${DRAFT_PREFIX}ann@example.com:Task:new`);
    expect(draftKey("ann@example.com", "Task", "")).toBe(draftKey("ann@example.com", "Task", "new"));
  });
});

describe("writeDraft / readDraft / clearDraft", () => {
  it("reads back what was written", () => {
    writeDraft(store, "k", draft());
    expect(readDraft(store, "k", NOW)).toEqual(draft());
  });

  it("has nothing to read when nothing was written", () => {
    expect(readDraft(store, "k", NOW)).toBeNull();
  });

  it("drops a draft older than the time to live, and forgets it", () => {
    writeDraft(store, "k", draft({ savedAt: NOW - DRAFT_TTL_MS - 1 }));
    expect(readDraft(store, "k", NOW)).toBeNull();
    expect(store.getItem("k")).toBeNull();
  });

  it("keeps a draft that is exactly at the edge of the time to live", () => {
    writeDraft(store, "k", draft({ savedAt: NOW - DRAFT_TTL_MS }));
    expect(readDraft(store, "k", NOW)).not.toBeNull();
  });

  it("ignores a corrupted entry instead of throwing", () => {
    store.setItem("k", "{not json");
    expect(readDraft(store, "k", NOW)).toBeNull();
  });

  it("clears one draft", () => {
    writeDraft(store, "k", draft());
    clearDraft(store, "k");
    expect(readDraft(store, "k", NOW)).toBeNull();
  });

  it("survives a storage that refuses to work, as in private mode", () => {
    const dead: DraftStore = {
      length: 0,
      key: () => { throw new Error("denied"); },
      getItem: () => { throw new Error("denied"); },
      setItem: () => { throw new Error("denied"); },
      removeItem: () => { throw new Error("denied"); },
    };
    expect(() => writeDraft(dead, "k", draft())).not.toThrow();
    expect(readDraft(dead, "k", NOW)).toBeNull();
    expect(() => clearDraft(dead, "k")).not.toThrow();
    expect(() => pruneDrafts(dead, NOW)).not.toThrow();
    expect(readDraft(null, "k", NOW)).toBeNull();
  });
});

describe("pruneDrafts", () => {
  it("sweeps expired drafts and leaves fresh ones and other keys alone", () => {
    writeDraft(store, `${DRAFT_PREFIX}a:Task:1`, draft({ savedAt: NOW - DRAFT_TTL_MS - 1 }));
    writeDraft(store, `${DRAFT_PREFIX}a:Task:2`, draft({ savedAt: NOW }));
    store.setItem("ddcore_lang", "pt-BR");

    pruneDrafts(store, NOW);

    expect(store.getItem(`${DRAFT_PREFIX}a:Task:1`)).toBeNull();
    expect(store.getItem(`${DRAFT_PREFIX}a:Task:2`)).not.toBeNull();
    expect(store.getItem("ddcore_lang")).toBe("pt-BR");
  });

  it("sweeps a draft it cannot parse", () => {
    store.setItem(`${DRAFT_PREFIX}a:Task:3`, "{not json");
    pruneDrafts(store, NOW);
    expect(store.getItem(`${DRAFT_PREFIX}a:Task:3`)).toBeNull();
  });
});

describe("draftDecision", () => {
  const server = { name: "TASK-1", subject: "saved", modified: "2026-01-01 10:00:00" };

  it("has nothing to do without a draft", () => {
    expect(draftDecision(null, server)).toBe("none");
  });

  it("restores a draft taken from the record as it still is", () => {
    expect(draftDecision(draft(), server)).toBe("restore");
  });

  it("asks when the record was saved by someone else after the draft", () => {
    expect(draftDecision(draft({ modified: "2026-01-01 09:00:00" }), server)).toBe("conflict");
  });

  it("has nothing to restore when the draft matches the record field by field", () => {
    expect(draftDecision(draft({ doc: { ...server } }), server)).toBe("none");
  });
});
