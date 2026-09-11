import "@ddcore/sdk/test";
import type { Project, User } from "../.ddcore/types";

const u = () => ddcore.utils;

describe("User extension", () => {
  it("grava o campo que o demo acrescentou ao User do core", () => {
    const project = ddcore.newDoc<Project>("Project", {
      code: "P-" + u().randomString(6),
      title: "Test project",
      assignee: "Administrator",
      start_date: u().today(),
    }).insert();

    const user = ddcore.newDoc<User>("User", {
      email: u().randomString(6) + "@x.com",
      full_name: "Ana",
      default_project: project.name,
    }).insert();

    expect(ddcore.db.getValue<string>("User", user.name, "default_project")).toBe(project.name);
  });

  it("o Link vale: um projeto inexistente é recusado", () => {
    expect(() =>
      ddcore.newDoc<User>("User", {
        email: u().randomString(6) + "@x.com",
        full_name: "Ze",
        default_project: "P-NAO-EXISTE",
      }).insert(),
    ).toThrow();
  });
});
