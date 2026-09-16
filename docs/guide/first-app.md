# Build Your First Application

This tutorial builds a small library catalogue with the published `ddcore` binary. You do not
need to clone or compile the framework, and you do not need Go or Node.js. You will:

1. create a project and an app;
2. define a **DocType** (`Book`) and migrate it into PostgreSQL;
3. create a record in the Desk;
4. add the DocType to the sidebar;
5. add a server-side validation rule;
6. test that rule.

> The TypeScript files on this page are read by the framework's acceptance suite
> (`internal/acceptance/quickstart_test.go`), which runs these steps against a freshly built
> binary. If this page and the binary disagree, the build fails.

---

## 1. Prerequisites

- macOS or Linux, on amd64 or arm64.
- `curl`.
- **PostgreSQL 14+** with an empty database you can connect to.

If you have Docker, this starts a suitable database:

```bash
docker run -d --name library-pg -p 5432:5432 \
  -e POSTGRES_USER=library -e POSTGRES_PASSWORD=library -e POSTGRES_DB=library \
  postgres:16
```

With an existing server, create a user and a database instead:

```sql
CREATE USER library WITH PASSWORD 'library';
CREATE DATABASE library OWNER library;
```

The rest of this page uses `postgres://library:library@localhost:5432/library?sslmode=disable`.
Replace it if your database differs.

---

## 2. Install the CLI

```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
ddcore version
```

The script installs to `/usr/local/bin` when that directory is writable and to
`~/.local/bin` otherwise. If `ddcore` is not found, add the directory to your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

---

## 3. Create the project and the app

```bash
mkdir my-library && cd my-library
ddcore init --dsn "postgres://library:library@localhost:5432/library?sslmode=disable"
ddcore new-app library
```

`ddcore init` writes two files:

- `ddcore.json`: the site's settings. It holds the database, the port (`8080`), the apps to
  load, and the default language and currency. Commit this file.
- `.env.example`: the environment variables a deployment can set. Copy it to `.env` (which you
  do not commit) for anything secret or specific to one machine, such as `DDCORE_DSN`.

`ddcore new-app library` creates `apps/library` and adds it to `apps` in `ddcore.json`:

```
apps/library/
  ddcore.app.ts       the app's definition: name, title, roles
  CLAUDE.md           conventions, for you and for a coding agent
  translations/       one CSV per language
  services/           business functions
```

The new site uses `"lang": "pt-BR"` and `"currency": "BRL"`. Change them in `ddcore.json` if
you want another default. Every string you write stays English, and the language only decides
how the Desk translates it.

---

## 4. Define a DocType

A DocType is a model: its fields become a table, a form, a list and REST endpoints. Each one
lives in its own folder under `doctypes/`.

Create `apps/library/doctypes/book/book.doctype.ts`:

```typescript
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Book",
  module: "Library",
  label: "Book",
  naming: { field: "isbn" },
  titleField: "title",
  trackChanges: true,
  fields: [
    { fieldname: "isbn", fieldtype: "Data", label: "ISBN", reqd: true, unique: true, inListView: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "author", fieldtype: "Data", label: "Author", reqd: true, inListView: true },
    { fieldname: "published_year", fieldtype: "Int", label: "Published Year" },
    {
      fieldname: "status",
      fieldtype: "Select",
      label: "Status",
      options: ["Available", "Checked Out", "Reserved"],
      default: "Available",
      inListView: true,
    },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
  ],
});
```

`naming: { field: "isbn" }` makes the ISBN the record's name, and `trackChanges` keeps a
version history. All field types are in the [Fieldtypes Reference](/agent/fieldtypes).

---

## 5. Migrate

```bash
ddcore migrate
```

`migrate` compares your DocTypes with the database and applies the difference in one
transaction. The first run also installs the framework's own tables (users, roles, files, jobs
and so on), so it prints several dozen statements. It ends with a line like:

```
ok: 69 DDL, 0 patches, apps installed: [core library]
```

It also generates `apps/library/.ddcore/types.d.ts`, the TypeScript types for your DocTypes.
Do not edit that file: `ddcore migrate` and `ddcore types` rewrite it. To see what a
migration would do without applying it, run `ddcore migrate --dry-run`.

---

## 6. Sign in to the Desk

Give the built-in `Administrator` account a password, then start the development server:

```bash
ddcore user passwd Administrator admin1234
ddcore dev
```

Open `http://localhost:8080/app/library/Book` and sign in as `Administrator` / `admin1234`.
That URL is the Book list: `/app/<workspace>/<DocType>`, where `library` stands in for a
workspace you will create in the next step. Click **New**, fill in
ISBN, Title and Author, and save. The record is stored in the `tab_book` table, with its
`owner`, `creation` and `modified` columns filled in.

The sidebar does not list Book yet. The next step adds it.

Leave `ddcore dev` running. It watches your files: saving a `.ts` file reloads the app, and a
DocType change is migrated automatically.

---

## 7. Add Book to the sidebar

The sidebar shows **workspaces**. A workspace groups what one area of work needs: the DocTypes,
reports and number cards, in the order you choose. The sidebar is always explicit: a DocType
does not appear there just because it exists.

Create `apps/library/workspaces/library.workspace.ts`:

```typescript
import { defineWorkspace } from "@ddcore/sdk";

export default defineWorkspace({
  name: "Library",
  label: "Library",
  icon: "list",
  roles: ["System Manager"],
  sidebar: [
    { label: "Books", doctype: "Book", icon: "notepad-text" },
  ],
  shortcuts: [
    { label: "Books", doctype: "Book", icon: "notepad-text" },
  ],
});
```

Then make it the workspace the Desk opens on. Replace `apps/library/ddcore.app.ts` with:

```typescript
import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: "library",
  title: "Library",
  roles: [],
  desk: { include: [], home: "Library" },
});
```

Reload the browser and open `http://localhost:8080/app`. It opens the Library workspace, with
**Books** in the sidebar and as a shortcut. Only users with one of the workspace's `roles` see
it. A sidebar entry can also point to a report (`report: "..."`), to any route
(`route: "/app/..."`), or have no link at all, in which case it is a heading. Number cards and
charts are in [Reports, workspaces, cards and charts](/agent/report-api).

---

## 8. Add a validation rule

Rules run on the server, inside the transaction that saves the document. Server code is
**synchronous**: no `await`, no Node.js APIs.

Create `apps/library/doctypes/book/book.controller.ts`:

```typescript
import { defineController, _ } from "@ddcore/sdk";
import type { Book } from "../../.ddcore/types";

export default defineController<Book>("Book", {
  validate(doc) {
    const currentYear = new Date().getFullYear();
    if (doc.published_year && doc.published_year > currentYear) {
      ddcore.throw(_("Published year cannot be in the future ({0}).", [doc.published_year]), {
        title: _("Invalid Publication Year"),
      });
    }
  },
});
```

`ddcore dev` reloads the app when you save. In the Desk, set a book's Published Year to `2099`
and save: the server refuses, nothing is written, and the Desk shows the message. The same
rule applies to the REST API, since every write goes through the controller.

`_()` marks a string for translation. Run `ddcore i18n extract --app library --lang pt-BR` to
add the new strings to `translations/pt-BR.csv` ([i18n](/agent/i18n)).

---

## 9. Test the rule

Create `apps/library/doctypes/book/book.test.ts`:

```typescript
import "@ddcore/sdk/test";
import type { Book } from "../../.ddcore/types";

describe("Book", () => {
  it("saves a book with a past publication year", () => {
    const book = ddcore.newDoc<Book>("Book", {
      isbn: "978-0131103627",
      title: "The C Programming Language",
      author: "Brian W. Kernighan, Dennis M. Ritchie",
      published_year: 1978,
    }).insert();

    expect(book.name).toBe("978-0131103627");
    expect(book.status).toBe("Available");
  });

  it("refuses a publication year in the future", () => {
    expect(() =>
      ddcore.newDoc<Book>("Book", {
        isbn: "978-9999999999",
        title: "A Book From the Future",
        author: "A. Traveller",
        published_year: 2099,
      }).insert(),
    ).toThrow();
  });
});
```

Run it:

```bash
ddcore test
```

```
2 tests, 0 failures
```

Each `it` runs in a transaction that is rolled back afterwards, so tests leave your
development data untouched.

---

## Next steps

- [Architecture](/guide/architecture): how documents, transactions and server code fit
  together.
- [Demo walkthrough](/guide/demo): links, child tables, reports and workspaces in a larger app.
- [Controller API](/agent/controller-api): every lifecycle hook and database method.
- [Deployment](/guide/deployment): running the app with `ddcore start`.
