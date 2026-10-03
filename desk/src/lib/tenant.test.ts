import { describe, expect, it } from "vitest";
import { tenantChoices, tenantLabel, tenantLinkState, tenantParam, type TenantBoot } from "./tenant";

const operator: TenantBoot = {
  id: "", title: "", platform: true,
  tenants: [{ id: "alfa", title: "Alfa Ltd", enabled: true }, { id: "beta", title: "", enabled: true }, { id: "off", title: "Off", enabled: false }],
};

describe("tenantLabel", () => {
  it("says nothing on a site without tenancy", () => {
    expect(tenantLabel(null, "Platform")).toBe("");
    expect(tenantLabel(undefined, "Platform")).toBe("");
  });
  it("names a tenant by its title, or its id when it has none", () => {
    expect(tenantLabel({ id: "alfa", title: "Alfa Ltd", platform: false }, "Platform")).toBe("Alfa Ltd");
    expect(tenantLabel({ id: "beta", title: "", platform: false }, "Platform")).toBe("beta");
  });
  it("names the platform space only to an operator", () => {
    expect(tenantLabel(operator, "Platform")).toBe("Platform");
    expect(tenantLabel({ id: "", title: "", platform: false }, "Platform")).toBe("");
  });
});

describe("tenantChoices", () => {
  it("offers nothing to someone who cannot enter a tenant", () => {
    expect(tenantChoices({ id: "alfa", title: "Alfa", platform: false }, "Platform")).toEqual([]);
    expect(tenantChoices(null, "Platform")).toEqual([]);
  });
  it("offers the platform space and every enabled tenant", () => {
    expect(tenantChoices(operator, "Platform")).toEqual([
      { id: "", label: "Platform", current: true },
      { id: "alfa", label: "Alfa Ltd", current: false },
      { id: "beta", label: "beta", current: false },
    ]);
  });
  it("marks the tenant the operator is in, and keeps it even when disabled", () => {
    const inside = { ...operator, id: "off", title: "Off" };
    const got = tenantChoices(inside, "Platform");
    expect(got.find((c) => c.current)?.id).toBe("off");
    expect(got[0]).toEqual({ id: "", label: "Platform", current: false });
  });
});

describe("tenantParam", () => {
  it("names the tenant the desk works in", () => {
    expect(tenantParam({ id: "alfa", title: "Alfa", platform: false })).toBe("alfa");
    expect(tenantParam({ ...operator, id: "beta" })).toBe("beta");
  });
  it("names none in the platform space or without tenancy", () => {
    expect(tenantParam(operator)).toBeNull();
    expect(tenantParam(null)).toBeNull();
  });
});

describe("tenantLinkState", () => {
  const alfaUser: TenantBoot = { id: "alfa", title: "Alfa", platform: false };
  it("asks nothing of a link without a tenant, or one naming where the desk already is", () => {
    expect(tenantLinkState(alfaUser, null)).toBe("ok");
    expect(tenantLinkState(alfaUser, "alfa")).toBe("ok");
    expect(tenantLinkState({ ...operator, id: "beta" }, "beta")).toBe("ok");
    expect(tenantLinkState(null, "alfa")).toBe("ok");
  });
  it("offers an operator to enter the tenant a link names", () => {
    expect(tenantLinkState(operator, "alfa")).toBe("enter");
    expect(tenantLinkState({ ...operator, id: "beta" }, "alfa")).toBe("enter");
  });
  it("turns away a tenant user, and an operator for a tenant that is disabled or unknown", () => {
    expect(tenantLinkState(alfaUser, "beta")).toBe("foreign");
    expect(tenantLinkState(operator, "off")).toBe("foreign");
    expect(tenantLinkState(operator, "nope")).toBe("foreign");
  });
});
