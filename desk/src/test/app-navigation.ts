// `$app/navigation` for the dom test project, which runs without the SvelteKit
// plugin. A test that navigates mocks this module with its own functions.
export const goto = async (_url: string, _opts?: unknown): Promise<void> => {};
