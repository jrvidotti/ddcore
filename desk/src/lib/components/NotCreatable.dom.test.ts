import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(args[+i])),
}));

import NotCreatable from "./NotCreatable.svelte";

function render(props: { label: string; description?: string; href: string }) {
  const target = document.createElement("div");
  const view = mount(NotCreatable, { target, props });
  flushSync();
  return { target, view };
}

describe("NotCreatable", () => {
  it("says the DocType is not created here, where it comes from, and links back to the list", () => {
    const { target, view } = render({ label: "Transfer", description: "Made with the Transfer button", href: "/app/payments/transfer" });
    const alert = target.querySelector("[role=alert]");
    expect(alert?.textContent).toContain("Transfer is not created here");
    expect(alert?.textContent).toContain("Made with the Transfer button");
    const back = target.querySelector("a.btn");
    expect(back?.getAttribute("href")).toBe("/app/payments/transfer");
    expect(back?.textContent).toBe("Back to Transfer");
    unmount(view);
  });
  it("leaves the description out when the DocType has none", () => {
    const { target, view } = render({ label: "Transfer", href: "/app/transfer" });
    expect(target.querySelectorAll("[role=alert] p")).toHaveLength(1);
    unmount(view);
  });
});
