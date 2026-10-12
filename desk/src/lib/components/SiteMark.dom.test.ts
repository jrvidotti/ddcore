import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it } from "vitest";
import { boot, type Boot } from "$lib/boot.svelte";
import SiteMark from "./SiteMark.svelte";

function render(site: Partial<Boot["site"]>, props: { size?: number; alt?: string } = {}) {
  boot.data = { site: { name: "Oblata", ...site } } as Boot;
  const target = document.createElement("div");
  const view = mount(SiteMark, { target, props });
  flushSync();
  return { target, view };
}

describe("SiteMark", () => {
  it("shows the app's logo image when it declares one (#128)", () => {
    const { target, view } = render({ brand: { logoImage: "/assets/site/logo?v=abc" } }, { size: 40, alt: "Oblata" });
    const img = target.querySelector("img");
    expect(img?.getAttribute("src")).toBe("/assets/site/logo?v=abc");
    expect(img?.getAttribute("alt")).toBe("Oblata");
    expect(img?.style.height).toBe("40px");
    unmount(view);
  });
  it("draws the letter square without one", () => {
    const { target, view } = render({ logo: "O" });
    expect(target.querySelector("img")).toBeNull();
    expect(target.querySelector(".site-mark")?.textContent).toBe("O");
    expect(target.querySelector(".site-mark")?.getAttribute("aria-hidden")).toBe("true");
    unmount(view);
  });
});
