import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import type { TenantBoot } from "$lib/tenant";

const state = vi.hoisted(() => ({ tenant: undefined as TenantBoot | undefined }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { get data() { return { site: { tenant: state.tenant } }; } },
}));
vi.mock("$lib/api", () => ({ api: { enterTenant: vi.fn() } }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));

import SharedNotice from "./SharedNotice.svelte";

function render(shared: boolean, tenant?: TenantBoot) {
  state.tenant = tenant;
  const target = document.createElement("div");
  const view = mount(SharedNotice, { target, props: { shared } });
  flushSync();
  return { target, view };
}

describe("SharedNotice", () => {
  it("tells an operator inside a tenant the DocType is read only, with a way back", () => {
    const { target, view } = render(true, { id: "demo", title: "Demo", platform: true });
    expect(target.querySelector(".shared-notice")?.textContent).toContain("Shared by every tenant: read only here");
    expect(target.querySelector("button")?.textContent).toContain("Go to the platform");
    unmount(view);
  });
  it("offers a tenant's own user no way out", () => {
    const { target, view } = render(true, { id: "demo", title: "Demo", platform: false });
    expect(target.querySelector(".shared-notice")).not.toBeNull();
    expect(target.querySelector("button")).toBeNull();
    unmount(view);
  });
  it("says nothing in the platform space, for a tenant's DocType, or without tenancy", () => {
    for (const [shared, tenant] of [
      [true, { id: "", title: "", platform: true }],
      [false, { id: "demo", title: "Demo", platform: true }],
      [true, undefined],
    ] as [boolean, TenantBoot | undefined][]) {
      const { target, view } = render(shared, tenant);
      expect(target.querySelector(".shared-notice")).toBeNull();
      unmount(view);
    }
  });
});
