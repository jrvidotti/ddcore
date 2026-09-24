// `$app/state` for the dom test project, which runs without the SvelteKit
// plugin. A test that reads the page mocks this module with its own values.
export const page = { params: {} as Record<string, string>, url: new URL("http://localhost/") };
