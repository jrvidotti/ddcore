# Third-party notices

DDCore's original code and documentation are licensed under the [MIT License](LICENSE).
Third-party components retain their respective licenses and copyrights. Their license
texts and copyright notices are available in their respective upstream repositories.
Redistribution must comply with those licenses.

## Frappe

[Frappe Framework](https://github.com/frappe/frappe) inspired DDCore's architecture
and DocType model. DDCore is an independent implementation, with no copied or
translated Frappe code, documentation, or assets, as confirmed by its maintainer.
DDCore is not affiliated with, sponsored by, or endorsed by Frappe Technologies.
Frappe's license and notices are available in its repository.

## Dependencies

DDCore uses [Go](https://github.com/golang/go), [goja](https://github.com/dop251/goja),
[esbuild](https://github.com/evanw/esbuild), [chi](https://github.com/go-chi/chi),
[pgx](https://github.com/jackc/pgx), [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk),
[cron](https://github.com/robfig/cron), [Svelte](https://github.com/sveltejs/svelte),
[SvelteKit](https://github.com/sveltejs/kit), and their dependencies.

See [go.mod](go.mod) and [desk/package-lock.json](desk/package-lock.json) for the
complete declared dependency lists and resolved versions. Consult each component's
upstream repository for its license and attribution requirements.
