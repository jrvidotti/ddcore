// The fixture the Go acceptance suite loads. It is not a showcase: the example
// app lives in its own repository (jrvidotti/ddcore-demo), the way a product
// app does. What stays here is only what internal/acceptance asserts on — two
// roles, a Project/Task pair with a child table, a workspace with a sidebar, a
// pt-BR catalogue and a `ddcore demo` seed — so that `go test ./internal/...`
// never depends on a checkout the framework does not own.
import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: "testapp",
  title: "Test App: Projects",
  version: "0.1.0",
  ddcore: ">=0.1.0 <1.0.0",
  roles: ["Project Manager", "Project Contributor"],
  scheduler: {
    daily: ["testapp.services.tasks.markOverdue"],
  },
  desk: {
    home: "Projects",
    include: ["client/lists.ts"],
    // an emoji, so the assertion covers a mark that is not one ASCII byte
    logo: "🧪",
  },
});
