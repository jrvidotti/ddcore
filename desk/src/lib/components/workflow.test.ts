import { describe, it, expect, vi, beforeEach } from "vitest";
import { FormController } from "../form.svelte";
import { api } from "../api";
import { toast, showError } from "../ui.svelte";
import { statusColor } from "../format";
import type { Meta } from "../meta";

vi.mock("../api", () => ({
  api: {
    post: vi.fn(),
    getDoc: vi.fn(),
    update: vi.fn(),
    insert: vi.fn(),
    docMethod: vi.fn(),
  },
}));

vi.mock("../ui.svelte", () => ({
  toast: vi.fn(),
  showError: vi.fn(),
  confirm: vi.fn(),
  ui: { busy: 0 },
}));

const catalogue: Record<string, string> = { Approve: "Aprovar" };

vi.mock("../boot.svelte", () => ({
  __: (s: string, args?: any[]) => {
    s = catalogue[s] ?? s;
    if (!args || !args.length) return s;
    return s.replace(/\{(\d+)\}/g, (_, i) => String(args[Number(i)] ?? ""));
  },
}));

vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const makeMeta = (opts: { submittable?: boolean; write?: boolean } = {}): Meta =>
  ({
    doctype: {
      name: "Artigo",
      label: "Artigo",
      submittable: opts.submittable ?? true,
      fields: [
        { fieldname: "titulo", fieldtype: "Data", label: "Título" },
        { fieldname: "conteudo", fieldtype: "Small Text", label: "Conteúdo" },
      ],
      formApps: [],
    },
    permissions: {
      read: true,
      write: opts.write ?? true,
      create: true,
      submit: true,
      cancel: true,
    },
  }) as unknown as Meta;

describe("Workflow form integration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("determines available workflow actions from doc._workflow", () => {
    const doc = {
      id: "PED-1",
      doctype: "Pedido",
      _workflow: {
        state: "Pending Approval",
        actions: [
          { action: "Approve", nextState: "Approved" },
          { action: "Reject", nextState: "Rejected" },
        ],
      },
    };
    expect(doc._workflow.actions).toHaveLength(2);
  });

  it("exposes workflow getter on FormController", () => {
    const meta = makeMeta();
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Draft",
        actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }],
      },
    };
    const frm = new FormController(meta, doc);
    expect(frm.workflow).toEqual(doc._workflow);
  });

  it("makes form and fields read-only when _workflow.allowEdit is false", () => {
    const meta = makeMeta({ write: true });
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Pending Approval",
        actions: [],
        allowEdit: false,
      },
    };
    const frm = new FormController(meta, doc);
    expect(frm.readOnly).toBe(true);
    expect(frm.isFieldEditable(meta.doctype.fields[0])).toBe(false);
  });

  it("makes form and fields read-only when workflow is present and write permission is denied", () => {
    const meta = makeMeta({ write: false });
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Draft",
        actions: [],
      },
    };
    const frm = new FormController(meta, doc);
    expect(frm.readOnly).toBe(true);
    expect(frm.isFieldEditable(meta.doctype.fields[0])).toBe(false);
  });

  it("keeps form editable when workflow allows edit and user has write permission", () => {
    const meta = makeMeta({ write: true });
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Draft",
        actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }],
        allowEdit: true,
      },
    };
    const frm = new FormController(meta, doc);
    expect(frm.readOnly).toBe(false);
    expect(frm.isFieldEditable(meta.doctype.fields[0])).toBe(true);
  });

  it("suppresses direct manual submit and cancel when workflow is present", async () => {
    const meta = makeMeta({ submittable: true, write: true });
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      docstatus: 0,
      _workflow: {
        state: "Draft",
        actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }],
      },
    };
    const frm = new FormController(meta, doc);
    expect(await frm.submit()).toBe(false);
    expect(await frm.cancel()).toBe(false);
    expect(api.docMethod).not.toHaveBeenCalled();
    expect(api.update).not.toHaveBeenCalled();
  });

  it("calls api.post to apply workflow transition and reloads document", async () => {
    const meta = makeMeta();
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Draft",
        actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }],
      },
    };
    const frm = new FormController(meta, doc);

    const updatedDoc = {
      ...doc,
      _workflow: {
        state: "Pending Approval",
        actions: [
          { action: "Approve", nextState: "Approved" },
          { action: "Reject", nextState: "Rejected" },
        ],
      },
    };
    vi.mocked(api.post).mockResolvedValue(updatedDoc);

    const ok = await frm.applyWorkflowAction("Submit for Approval");
    expect(ok).toBe(true);
    expect(api.post).toHaveBeenCalledWith("/api/workflow/apply", {
      doctype: "Artigo",
      id: "ART-001",
      action: "Submit for Approval",
    });
    expect(frm.workflow?.state).toBe("Pending Approval");
    expect(frm.workflow?.actions).toHaveLength(2);
    expect(toast).toHaveBeenCalledWith(
      expect.stringContaining("Submit for Approval"),
      expect.objectContaining({ indicator: "green" }),
    );
  });

  it("names the applied action in the user's language", async () => {
    const doc = { id: "ART-001", doctype: "Artigo", _workflow: { state: "Pending Approval", actions: [{ action: "Approve", nextState: "Approved" }] } };
    const frm = new FormController(makeMeta(), doc);
    vi.mocked(api.post).mockResolvedValue({ ...doc, _workflow: { state: "Approved", actions: [] } });
    expect(await frm.applyWorkflowAction("Approve")).toBe(true);
    expect(toast).toHaveBeenCalledWith("Action 'Aprovar' applied", expect.anything());
  });

  it("refuses to apply an action over unsaved edits", async () => {
    const doc = { id: "ART-001", doctype: "Artigo", titulo: "a", _workflow: { state: "Draft", actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }] } };
    const frm = new FormController(makeMeta(), doc);
    frm.doc.titulo = "edited";
    expect(frm.isDirty).toBe(true);
    expect(await frm.applyWorkflowAction("Submit for Approval")).toBe(false);
    expect(api.post).not.toHaveBeenCalled();
  });

  it("handles workflow transition errors gracefully", async () => {
    const meta = makeMeta();
    const doc = {
      id: "ART-001",
      doctype: "Artigo",
      _workflow: {
        state: "Draft",
        actions: [{ action: "Submit for Approval", nextState: "Pending Approval" }],
      },
    };
    const frm = new FormController(meta, doc);
    vi.mocked(api.post).mockRejectedValue(new Error("Workflow error"));

    const ok = await frm.applyWorkflowAction("Submit for Approval");
    expect(ok).toBe(false);
    expect(showError).toHaveBeenCalled();
  });

  it("resolves state pill indicator color for canonical workflow states", () => {
    expect(statusColor("Draft")).toBe("orange");
    expect(statusColor("Pending Approval")).toBe("orange");
    expect(statusColor("Approved")).toBe("green");
    expect(statusColor("Rejected")).toBe("red");
  });

  it("determines action button display strategy (1 or 2 buttons vs dropdown)", () => {
    const getActionDisplayMode = (actions: { action: string }[]) => {
      if (!actions || actions.length === 0) return "none";
      if (actions.length <= 2) return "buttons";
      return "dropdown";
    };

    expect(getActionDisplayMode([])).toBe("none");
    expect(getActionDisplayMode([{ action: "Submit for Approval" }])).toBe("buttons");
    expect(
      getActionDisplayMode([{ action: "Approve" }, { action: "Reject" }]),
    ).toBe("buttons");
    expect(
      getActionDisplayMode([
        { action: "Approve" },
        { action: "Reject" },
        { action: "Request Changes" },
      ]),
    ).toBe("dropdown");
  });
});
