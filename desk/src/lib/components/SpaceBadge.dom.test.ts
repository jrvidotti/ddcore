import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import { tenantLabel, type TenantBoot } from "$lib/tenant";

const state = vi.hoisted(() => ({ tenant: undefined as TenantBoot | undefined }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { get data() { return { site: { tenant: state.tenant } }; } },
  spaceLabel: () => tenantLabel(state.tenant, "Platform"),
}));
vi.mock("$lib/api", () => ({ api: { enterTenant: vi.fn() } }));

import SpaceBadge from "./SpaceBadge.svelte";

const tenants = [{ id: "demo", title: "Demo", enabled: true }];

function render(tenant?: TenantBoot) {
  state.tenant = tenant;
  const target = document.createElement("div");
  document.body.appendChild(target);
  const view = mount(SpaceBadge, { target });
  flushSync();
  return { target, done: () => { unmount(view); target.remove(); } };
}

describe("SpaceBadge", () => {
  it("shows an operator the platform in its own color, and opens the tenant menu", () => {
    const { target, done } = render({ id: "", title: "", platform: true, tenants });
    const btn = target.querySelector<HTMLButtonElement>("button.space.platform");
    expect(btn?.textContent).toContain("Platform");
    expect(target.querySelector(".tenant-menu")).toBeNull();
    btn!.click();
    flushSync();
    expect(target.querySelector(".tenant-menu")?.textContent).toContain("Demo");
    done();
  });
  it("shows an operator inside a tenant the tenant's title", () => {
    const { target, done } = render({ id: "demo", title: "Demo", platform: true, tenants });
    expect(target.querySelector("button.space.tenant")?.textContent).toContain("Demo");
    done();
  });
  it("shows a tenant's own user the tenant, with nothing to click", () => {
    const { target, done } = render({ id: "demo", title: "", platform: false });
    expect(target.querySelector("span.space.tenant")?.textContent).toContain("demo");
    expect(target.querySelector("button")).toBeNull();
    done();
  });
  it("draws nothing without tenancy, nor for a platform user who is not an operator", () => {
    for (const t of [undefined, { id: "", title: "", platform: false }]) {
      const { target, done } = render(t);
      expect(target.querySelector(".space-badge")).toBeNull();
      done();
    }
  });
});
