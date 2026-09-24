import { afterEach, describe, expect, it, vi } from "vitest";
import { api, type ToDoDoc, type PendingWorkPage } from "./api";
import { DocAssignments, PendingWork, TODO_WINDOW_LIMIT, pendingTasks, refreshPendingCount } from "./assignments.svelte";

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

  it("sends the filters, the order and the page size", async () => {
    const pendingSpy = vi.spyOn(api.assignments, "pending").mockResolvedValue({ data: [], total: 0 });
    const pw = new PendingWork();
    pw.limit = 50;
    pw.filters = { q: "leite", priority: "High", due: "", date: "", user: "bia@x.com" };
    pw.orderBy = "priority desc";

    await pw.load(50, "Open", "assigned_by_me");

    expect(pendingSpy).toHaveBeenCalledWith({
      limit: 50, offset: 50, status: "Open", scope: "assigned_by_me",
      q: "leite", priority: "High", user: "bia@x.com", order_by: "priority desc",
    });
  });

  it("loads a window unpaged for the calendar", async () => {
    const pendingSpy = vi.spyOn(api.assignments, "pending").mockResolvedValue({ data: [], total: 0 });
    const pw = new PendingWork();
    pw.window = { date_from: "2026-08-30", date_to: "2026-10-10" };

    await pw.load(40);

    expect(pendingSpy).toHaveBeenCalledWith(expect.objectContaining({
      limit: TODO_WINDOW_LIMIT, offset: 0, date_from: "2026-08-30", date_to: "2026-10-10",
    }));
  });

  it("loads every status for the Kanban", async () => {
    const pendingSpy = vi.spyOn(api.assignments, "pending").mockResolvedValue({ data: [], total: 0 });
    const pw = new PendingWork();
    pw.window = { allStatuses: true };

    await pw.load(0, "Open");

    expect(pendingSpy).toHaveBeenCalledWith({ limit: TODO_WINDOW_LIMIT, offset: 0, status: "all", scope: "assigned_to_me" });
  });

  it("moves a task through the matching endpoint", async () => {
    vi.spyOn(api.assignments, "pending").mockResolvedValue({ data: [sampleTodo], total: 1 });
    const complete = vi.spyOn(api.assignments, "complete").mockResolvedValue({ ...sampleTodo, status: "Closed" });
    const revoke = vi.spyOn(api.assignments, "revoke").mockResolvedValue({ success: true });
    const reopen = vi.spyOn(api.assignments, "reopen").mockResolvedValue(sampleTodo);
    const pw = new PendingWork();
    await pw.load(0);

    await pw.setStatus(sampleTodo.id, "Closed");
    pw.rows = [{ ...sampleTodo, status: "Closed" }];
    await pw.setStatus(sampleTodo.id, "Open");
    await pw.setStatus(sampleTodo.id, "Cancelled");

    expect(complete).toHaveBeenCalledWith(sampleTodo.id);
    expect(reopen).toHaveBeenCalledWith(sampleTodo.id);
    expect(revoke).toHaveBeenCalledWith(sampleTodo.id);
  });

  it("puts the task back when the server refuses the move", async () => {
    vi.spyOn(api.assignments, "pending").mockResolvedValue({ data: [sampleTodo], total: 1 });
    vi.spyOn(api.assignments, "complete").mockRejectedValue(new Error("nope"));
    const pw = new PendingWork();
    await pw.load(0);

    await expect(pw.setStatus(sampleTodo.id, "Closed")).rejects.toThrow("nope");

    expect(pw.rows[0].status).toBe("Open");
    expect(pw.pending).toBe("");
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
