# Feature requests

ddcore is a **host, not a library**: an app is TypeScript that the binary reads and runs, and there
is nowhere to compile Go into it. A capability the core does not have is therefore not something an
app can add — it can only be worked around, and the workaround is app code standing in for
framework code. The next step, when that happens, is an issue at
https://github.com/jrvidotti/ddcore/issues.

## What belongs upstream

A request is worth opening when the gap is the framework's, not this app's:

- **it completes something the core already owns**, so several apps stop writing the same code — the
  roadmap's first criterion is exactly that, favouring what completes an existing abstraction or
  removes repeated app code;
- **the workaround puts presentation in the schema.** The worked case is issue #1: with no
  field-level button, an app declared four `HTML` fields in the DocType only to give four buttons
  somewhere to live, and a field holding no data became part of the document's
  definition;
- **the workaround re-implements what the core owns.** The same issue had every app writing its own
  `esc()` against `{@html}` (one forgotten call is an XSS), its own event delegation and its own
  label translation — while `hidden`, `readOnly`, `dependsOn` and the permission check could not
  see the button at all;
- **a documented gap that an app actually hit** — `auth`'s "Not here yet", the Single operations
  `controller-api` lists as unsupported, the host call `ops` describes as non-interruptible;
- **the core refuses by design and offers no route.** `extending` lists what is not overridable at
  all and `conventions` lists what is refused rather than guessed; a refusal with no alternative for
  a legitimate case is a gap in the alternative, not in the refusal;
- **the meta accepts what the desk does not render** — a field property or a fieldtype that
  typechecks and migrates but has no control behind it.

The test that settles most cases: **would another app need the same thing?** If only this domain
cares, it is a service.

## What does not

- a rule belonging to one domain — it goes in `services/*.ts` and is called from the controller;
- anything already reachable from a controller, a service, `docEvents`, `desk.include` or
  `defineListView`; see `controller-api`, `form-api` and `extending`;
- widening what an extension may override: an extension restricts what the host allows and never
  widens it, which is the design and not an omission;
- an app's own labels, naming or translations; see `i18n`;
- behaviour that contradicts what these documents promise — that is a bug report, and it needs a
  reproduction rather than a proposal;
- "Frappe has it", with no case behind it. The port is selective by intent.

## Before opening one

- **Search the issues, open and closed**, by the capability's name rather than by the symptom:
  `gh issue list --repo jrvidotti/ddcore --state all --search "<capability>"`. If one already
  describes it, add the case as a comment instead of opening a second issue — a second app needing
  the same thing is what moves an item up the roadmap.
- **Read the roadmap.** `ROADMAP.md` and `docs/frappe-port-inventory.md` track the known gaps
  under inventory IDs (`SEC-05`, `OPS-02`, …). Neither ships inside the binary, so they are read on
  GitHub; cite the ID when the gap is already listed.
- **Confirm it is really missing.** `ddcore docs <name>` (or `ddcore://docs/<name>`) and the
  generated `.ddcore/types.d.ts` are the authority on what the installed version exposes.
- **Check the version.** `ddcore version` against the app's `requires: { ddcore: ">=…" }` — the
  capability may already exist in a release newer than the one installed.
- **Write the workaround first.** The app has to keep shipping, and the workaround is the request's
  strongest evidence: it is what the issue reports under *What an app has to do instead*.

## What a good request contains

Issue #1 is the template, and it was implemented:

```
## What is missing                 the capability, and what the core offers instead
## Where it came up                the screen or the rule that needed it — one concrete case
## What an app has to do instead   the workaround, in code, and what it costs
## Proposal                        the API as it would be called, in TypeScript
```

The title names the capability, not the symptom ("Field-level button: an action attached to an
input", not "buttons are hard"). The label is `enhancement`. A proposal carrying a signature a
reviewer can argue with is worth more than a description of the problem alone.

## The agent does not open it alone

Opening an issue publishes to a public repository under the user's account. An agent runs the checks
above and **drafts** the issue in full, then asks. Only with an explicit yes does it run:

```bash
gh issue create --repo jrvidotti/ddcore --label enhancement \
  --title "<capability>" --body-file <draft>
```

If the answer is no, the draft stays in the app's repository and the workaround stands.
