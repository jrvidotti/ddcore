# Contributing to DDCore

Read [DEVELOPMENT.md](DEVELOPMENT.md), [AGENTS.md](AGENTS.md), and the
[app API reference](docs/agent/index.md) before making changes. Keep changes focused,
explain their purpose, and include relevant verification results in your pull request.
Run `make check` and `make test` before submitting. Write code comments, documentation,
and user-facing source strings in English; translate interface strings as described
in the development guide.

## Contribution license

By submitting an original contribution for inclusion in DDCore, you agree to license
it under the project's [MIT License](LICENSE). You retain your copyright. Submit only
material you have the right to contribute. No copyright assignment, separate CLA, or
mandatory signed-off-by trailer is required.

## Third-party material

Identify copied, adapted, or translated code, documentation, and assets in your pull
request. Record the upstream URL, exact version or commit, affected DDCore files,
license, and original copyright notices. Preserve existing notices in the source and
record the upstream license link and attribution in
`THIRD_PARTY_NOTICES.md`. Include additional notices wherever required by the
applicable license. Acknowledgment alone does not replace license conditions.

Review each source separately: Frappe Framework, ERPNext, other applications,
documentation, and branding do not necessarily share a license. Translation between
programming languages does not remove upstream obligations. Do not label third-party
material MIT unless its license permits that treatment; resolve compatibility before
incorporating it. References to products must not imply affiliation or endorsement.

## Dependency and release maintenance

When updating dependencies or the Go toolchain, review the versions and complete
upstream license/notice links in `THIRD_PARTY_NOTICES.md`. Review nested notices
and preserve copyright statements as required by the applicable licenses. Inspect the modules
used by `go list -deps ./cmd/ddcore` for all supported GOOS/GOARCH targets. For the
desk, run `npm run build -- --sourcemap` and inspect client source maps for bundled
packages, including transitive dependencies and generated runtime helpers; rebuild
normally afterward. Build-only tools are not automatically part of the distributed
binary. Do not assume the npm `devDependencies` flag means a package is not bundled.

Verify that release archives and the final container image retain the legal
documents. The CLI installer installs only the executable; redistributors must retain
the documents from the corresponding release archive. The notices describe the recorded distribution and are not a legal
audit or a license grant for unrelated third-party material.
