import { describe, expect, it } from "vitest";
import { hashColor } from "../../format";
import { avatarColor, initials } from "./card-avatar";

describe("card avatar", () => {
  it.each([
    ["MARIA DA SILVA", "MS"],
    ["maria da silva", "MS"],
    ["Acme", "A"],
    ["  (Ana) Souza. ", "AS"],
    ["élodie", "É"],
    ["élodie durand", "ÉD"],
    ["Zoë 🙂 Ødegaard", "ZØ"],
    ["42 Street", "4S"],
    ["", ""],
    ["  ... ", ""],
  ])("takes the initials of %j as %j", (title, want) => {
    expect(initials(title)).toBe(want);
  });

  it("colours the same title the same way every time", () => {
    expect(avatarColor("Maria da Silva")).toBe(avatarColor("Maria da Silva"));
    expect(["blue", "green", "orange", "red", "purple", "gray"]).toContain(avatarColor("Maria da Silva"));
  });

  it("hashes words a status would colour canonically", () => {
    // "Open" is blue as a status; as a name it is just text
    expect(avatarColor("Open")).toBe(hashColor("Open"));
  });
});
