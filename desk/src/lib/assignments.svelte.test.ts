import { afterEach, describe, expect, it, vi } from "vitest";
import { api, type ToDoDoc, type PendingWorkPage } from "./api";
import { DocAssignments, PendingWork, pendingTasks, refreshPendingCount } from "./assignments.svelte";

const sampleTodo: ToDoDoc = {
  id: "TODO-0001",
  status: "Open",
  priority: "Medium",
  allocated_to: "ana@x.com",
  assigned_by: "admin@x.com",
  description: "Please review document",
  reference_type: "Pessoa",
  reference_id: "Cliente A",
  date: "2026-09-15",
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("DocAssignments", () => {
  it("loads assignments for a document", async () => {
    const forDoc = vi.spyOn(api.assignments, "forDoc").mockResolvedValue([sampleTodo]);
    const docAssign = new DocAssignments("Pessoa", "Cliente A");

    await docAssign.load();

    expect(forDoc).toHaveBeenCalledWith("Pessoa", "Cliente A");
    expect(docAssign.rows).toEqual([sampleTodo]);
    expect(docAssign.loading).toBe(false);
  });

  it("assigns user to document and reloads", async () => {
    const assignSpy = vi.spyOn(api.assignments, "assign").mockResolvedValue(sampleTodo);
    const forDoc = vi.spyOn(api.assignments, "forDoc").mockResolvedValue([sampleTodo]);
    const docAssign = new DocAssignments("Pessoa", "Cliente A");

    await docAssign.assign({ allocated_to: "ana@x.com", description: "Review" });

    expect(assignSpy).toHaveBeenCalledWith("Pessoa", "Cliente A", { allocated_to: "ana@x.com", description: "Review" });
    expect(forDoc).toHaveBeenCalledWith("Pessoa", "Cliente A");
    expect(docAssign.rows).toEqual([sampleTodo]);
  });

  it("completes assignment and updates local row", async () => {
    vi.spyOn(api.assignments, "forDoc").mockResolvedValue([sampleTodo]);
    const completeSpy = vi.spyOn(api.assignments, "complete").mockResolvedValue({ ...sampleTodo, status: "Closed" });
    const docAssign = new DocAssignments("Pessoa", "Cliente A");
    await docAssign.load();

    await docAssign.complete("TODO-0001");

    expect(completeSpy).toHaveBeenCalledWith("TODO-0001");
    expect(docAssign.rows[0].status).toBe("Closed");
  });

  it("revokes assignment and removes or updates local row", async () => {
    vi.spyOn(api.assignments, "forDoc").mockResolvedValue([sampleTodo]);
    const revokeSpy = vi.spyOn(api.assignments, "revoke").mockResolvedValue({ success: true });
    const docAssign = new DocAssignments("Pessoa", "Cliente A");
    await docAssign.load();

    await docAssign.revoke("TODO-0001");

    expect(revokeSpy).toHaveBeenCalledWith("TODO-0001");
    expect(docAssign.rows.length).toBe(0);
  });

  it("handles errors during load", async () => {
    vi.spyOn(api.assignments, "forDoc").mockRejectedValue(new Error("Access forbidden"));
    const docAssign = new DocAssignments("Pessoa", "Cliente A");

    await docAssign.load();

    expect(docAssign.error).toBe("Access forbidden");
    expect(docAssign.rows).toEqual([]);
    expect(docAssign.loading).toBe(false);
  });
});

describe("PendingWork", () => {
  it("loads pending work with pagination and filters", async () => {
    const pendingSpy = vi.spyOn(api.assignments, "pending").mockResolvedValue({
      data: [sampleTodo],
      total: 1,
    });
    const pw = new PendingWork();

    await pw.load(0, "Open", "assigned_to_me");

    expect(pendingSpy).toHaveBeenCalledWith({
      limit: 20,
      offset: 0,
      status: "Open",
      scope: "assigned_to_me",
    });
    expect(pw.rows).toEqual([sampleTodo]);
    expect(pw.total).toBe(1);
    expect(pw.loading).toBe(false);
  });

  it("loads pending work with status 'all'", async () => {
    const pendingSpy = vi.spyOn(api.assignments, "pending").mockResolvedValue({
      data: [sampleTodo],
      total: 1,
    });
    const pw = new PendingWork();

    await pw.load(0, "all", "assigned_to_me");

    expect(pendingSpy).toHaveBeenCalledWith({
      limit: 20,
      offset: 0,
      status: "all",
      scope: "assigned_to_me",
    });
  });

  it("refreshes pending task counter badge", async () => {
    vi.spyOn(api.assignments, "pending").mockResolvedValue({
      data: [sampleTodo],
      total: 5,
    });

    await refreshPendingCount();

    expect(pendingTasks.count).toBe(5);
  });

  it("ignores stale responses after destroy", async () => {
    let resolve!: (page: PendingWorkPage) => void;
    vi.spyOn(api.assignments, "pending").mockReturnValue(new Promise((r) => { resolve = r; }));
    const pw = new PendingWork();

    const pendingPromise = pw.load(0, "Open", "assigned_to_me");
    pw.destroy();
    resolve({ data: [sampleTodo], total: 1 });
    await pendingPromise;

    expect(pw.rows).toEqual([]);
  });
});
