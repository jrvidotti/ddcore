import { describe, expect, it } from "vitest";
import { thumbUrl } from "./thumbs";

describe("thumbUrl", () => {
  it("rounds up to the sizes the server makes", () => {
    expect(thumbUrl("/files/a.png", 40)).toBe("/files/a.png?w=64");
    expect(thumbUrl("/files/a.JPG", 128)).toBe("/files/a.JPG?w=128");
    expect(thumbUrl("/files/a.webp", 129)).toBe("/files/a.webp?w=256");
    expect(thumbUrl("/files/a.gif", 4000)).toBe("/files/a.gif?w=512");
  });
  it("leaves anything but a public image alone", () => {
    expect(thumbUrl("/private/files/a.png", 64)).toBe("/private/files/a.png");
    expect(thumbUrl("/files/a.pdf", 64)).toBe("/files/a.pdf");
    expect(thumbUrl("https://x.com/a.png", 64)).toBe("https://x.com/a.png");
    expect(thumbUrl("/files/a.png?w=64", 128)).toBe("/files/a.png?w=64");
    expect(thumbUrl(null, 64)).toBe("");
  });
  it("refuses a width that is no size", () => {
    expect(() => thumbUrl("/files/a.png", 0)).toThrow();
  });
});
