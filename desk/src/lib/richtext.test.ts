// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { htmlToLine, htmlToText, isEmptyRichText, normalizeRichText, sanitizeHtml } from "./richtext";

// The same file the Go tests read. The two sanitizers are different libraries,
// so what is asserted byte for byte is what they must agree on: how a stored
// value reads, and what never survives.
const fixture = JSON.parse(
  // vitest runs from `desk/`, next to the Go tree that owns the fixture
  readFileSync(resolve(process.cwd(), "../internal/richtext/testdata/cases.json"), "utf8"),
);

describe("the shared fixture", () => {
  for (const c of fixture.normalize) {
    it(`normalize: ${c.name}`, () => expect(normalizeRichText(c.in)).toBe(c.out));
  }
  for (const c of fixture.text) {
    it(`text: ${c.name}`, () => expect(htmlToText(c.in)).toBe(c.out));
  }
  for (const c of fixture.contains) {
    it(`contains: ${c.name}`, () => {
      const got = normalizeRichText(c.in);
      for (const want of c.present) {
        if (want.startsWith("target=") || want === "noopener" || want === "noreferrer" || want === "nofollow") {
          continue; // the editor, not the desk's sanitizer, writes these
        }
        expect(got).toContain(want);
      }
    });
  }
  for (const c of fixture.strip) {
    it(`strip: ${c.name}`, () => {
      const got = sanitizeHtml(c.in).toLowerCase();
      for (const absent of c.absent) expect(got).not.toContain(absent.toLowerCase());
    });
  }
});

describe("sanitizeHtml", () => {
  it("keeps what the editor writes", () => {
    const got = sanitizeHtml(`<h2>T</h2><p><strong>b</strong></p><pre><code class="language-ts">x</code></pre><img src="/files/a.png" alt="A">`);
    expect(got).toContain("<h2>T</h2>");
    expect(got).toContain('class="language-ts"');
    expect(got).toContain('src="/files/a.png"');
  });

  it("drops an image the server would not store", () => {
    expect(sanitizeHtml(`<img src="https://evil.example/p.gif">`)).not.toContain("evil.example");
    expect(sanitizeHtml(`<img src="/files/../../etc/passwd">`)).not.toContain("src=");
  });

  it("drops a class that is not a code language", () => {
    expect(sanitizeHtml(`<p class="hidden">x</p>`)).not.toContain("class=");
  });
});

describe("normalizeRichText", () => {
  it("reads a legacy value as the plain text it is", () => {
    expect(normalizeRichText("a < b")).toBe("<p>a &lt; b</p>");
    expect(normalizeRichText("one\ntwo")).toBe("<p>one<br>two</p>");
  });
  it("is idempotent", () => {
    for (const v of ["plain", "a < b", "<p>markup</p>", "one\n\ntwo"]) {
      expect(normalizeRichText(normalizeRichText(v))).toBe(normalizeRichText(v));
    }
  });
});

describe("isEmptyRichText", () => {
  it("sees what an emptied editor leaves", () => {
    for (const v of ["", "<p></p>", "<p><br></p>"]) expect(isEmptyRichText(v)).toBe(true);
    expect(isEmptyRichText("<p>x</p>")).toBe(false);
    expect(isEmptyRichText('<p><img src="/files/a.png"></p>')).toBe(false);
  });
});

describe("htmlToLine", () => {
  it("puts a whole value on one line", () => {
    expect(htmlToLine("<p>one</p><p>two</p>")).toBe("one two");
  });
});
