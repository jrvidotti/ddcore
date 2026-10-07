import { describe, expect, it } from "vitest";
import { loginMethods, pickMethod, pickTenant } from "./login-methods";

describe("loginMethods", () => {
  it("offers the e-mail form and each credential provider", () => {
    const m = loginMethods({ password: true, credentials: [{ id: "tagone", label: "TagOne" }] }, "E-mail");
    expect(m.map((x) => x.key)).toEqual(["password", "cred:tagone"]);
    expect(m[1]).toMatchObject({ kind: "credential", id: "tagone", label: "TagOne" });
  });
  it("leaves the e-mail form out when password sign-in is off", () => {
    expect(loginMethods({ password: false, credentials: [{ id: "x", label: "X" }] }, "E-mail").map((x) => x.key)).toEqual(["cred:x"]);
  });
  it("is just the e-mail form on a site without providers", () => {
    expect(loginMethods(undefined, "E-mail").map((x) => x.key)).toEqual(["password"]);
  });
});

describe("pickMethod", () => {
  const methods = loginMethods({ credentials: [{ id: "a", label: "A" }] }, "E-mail");
  it("reopens the method used last time", () => expect(pickMethod(methods, "cred:a")).toBe("cred:a"));
  it("ignores a method no longer offered", () => expect(pickMethod(methods, "cred:gone")).toBe("password"));
  it("falls back to the first", () => expect(pickMethod(methods, null)).toBe("password"));
});

describe("pickTenant", () => {
  it("keeps the remembered tenant while it is listed", () => {
    expect(pickTenant([{ id: "a" }, { id: "b" }], "b")).toBe("b");
    expect(pickTenant([{ id: "a" }, { id: "b" }], "c")).toBe("");
  });
  it("preselects the only tenant", () => expect(pickTenant([{ id: "a" }], null)).toBe("a"));
});
