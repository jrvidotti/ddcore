import { afterEach, describe, expect, it, vi } from "vitest";
import { api, type DocShare } from "./api";
import { DocSharesState } from "./shares.svelte";

const share: DocShare = {
  id: "abc", user: "ana@x.com", share_doctype: "Pessoa", share_id: "Cliente A",
  read: true, write: false, share: false, override_scope: false, owner: "admin@x.com",
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("DocSharesState", () => {
  it("loads the shares and what the user may do with them", async () => {
    const forDoc = vi.spyOn(api.shares, "forDoc").mockResolvedValue({ shares: [share], canShare: true, canOverrideScope: false });
    const state = new DocSharesState("Pessoa", "Cliente A");

    await state.load();

    expect(forDoc).toHaveBeenCalledWith("Pessoa", "Cliente A");
    expect(state.rows).toEqual([share]);
    expect(state.canShare).toBe(true);
    expect(state.canOverrideScope).toBe(false);
    expect(state.loading).toBe(false);
  });

  it("hides sharing when the list cannot be loaded", async () => {
    vi.spyOn(api.shares, "forDoc").mockRejectedValue(new Error("nope"));
    const state = new DocSharesState("Pessoa", "Cliente A");

    await state.load();

    expect(state.rows).toEqual([]);
    expect(state.canShare).toBe(false);
    expect(state.error).toBe("nope");
  });

  it("adds a share and reloads", async () => {
    const add = vi.spyOn(api.shares, "add").mockResolvedValue(share);
    vi.spyOn(api.shares, "forDoc").mockResolvedValue({ shares: [share], canShare: true, canOverrideScope: true });
    const state = new DocSharesState("Pessoa", "Cliente A");

    await state.add({ user: "ana@x.com", write: true });

    expect(add).toHaveBeenCalledWith("Pessoa", "Cliente A", { user: "ana@x.com", write: true });
    expect(state.rows).toEqual([share]);
  });

  it("removes a share", async () => {
    const remove = vi.spyOn(api.shares, "remove").mockResolvedValue({ ok: true });
    const state = new DocSharesState("Pessoa", "Cliente A");
    state.rows = [share];

    await state.remove("ana@x.com");

    expect(remove).toHaveBeenCalledWith("Pessoa", "Cliente A", "ana@x.com");
    expect(state.rows).toEqual([]);
    expect(state.pending).toBe("");
  });
});
