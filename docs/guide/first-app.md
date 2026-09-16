# Build Your First Application

> **Documentation note:** This guide describes developing an outside application with `ddcore` without cloning or compiling the framework source repository. It targets the latest published binary release on macOS and Linux.

This tutorial guides you through creating a complete standalone business application with **ddcore** (*Data Driven Core*): configuring PostgreSQL, initializing an app directory, defining a **DocType**, running database migrations, managing users, creating records in the Desk, and adding server-side validation.

---

## 1. Prerequisites

Before beginning, ensure you have:

1. **PostgreSQL 14+** running and reachable.
2. A PostgreSQL user with database creation rights (or a pre-created empty database).
3. `curl` and `sh` (available on standard macOS and Linux distributions).

Verify PostgreSQL access:

```bash
psql -U postgres -c "SELECT version();"
```

Create a dedicated database and user for your application:

```sql
CREATE USER myapp WITH PASSWORD 'myappsecret';
CREATE DATABASE myapp_dev OWNER myapp;
```

---

## 2. Install the `ddcore` CLI

Install the standalone CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
```

Verify that `ddcore` is in your `PATH` and executable:

```bash
ddcore --version
```

If the command is not found, verify that `~/.local/bin` (or `/usr/local/bin`) is in your environment's `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

---

## 3. Initialize Your Application Project

Create a new directory for your project and initialize it:

```bash
mkdir my-library && cd my-library
ddcore init
```

This creates a `ddcore.json` configuration file:

```json
{
  "apps": ["library"],
  "db": {
    "dsn": "postgres://myapp:myappsecret@localhost:5432/myapp_dev?sslmode=disable"
  }
}
```

Now create your application module:

```bash
ddcore new-app library
```

This scaffolds the `library` directory containing:
- `library/doctypes/`: Directory where your DocTypes will reside.
- `library/tsconfig.json`: TypeScript compiler configuration for strict type checking.

---

## 4. Define a DocType

DocTypes are declared in TypeScript files ending in `.doctype.ts`.

Create a directory for the `Book` DocType:

```bash
mkdir -p library/doctypes/book
```

Create `library/doctypes/book/book.doctype.ts`:

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
    {
      fieldname: "isbn",
      fieldtype: "Data",
      label: "ISBN",
      reqd: true,
      unique: true,
      inListView: true,
    },
    {
      fieldname: "title",
      fieldtype: "Data",
      label: "Title",
      reqd: true,
      inListView: true,
    },
    {
      fieldname: "author",
      fieldtype: "Data",
      label: "Author",
      reqd: true,
      inListView: true,
    },
    {
      fieldname: "published_year",
      fieldtype: "Int",
      label: "Published Year",
    },
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
    {
      role: "System Manager",
      read: true,
      write: true,
      create: true,
      delete: true,
      report: true,
      export: true,
    },
  ],
});
```

For complete field types and configuration options, see the [Fieldtypes Reference](/agent/fieldtypes).

---

## 5. Run Database Migrations & Generate Types

Run `migrate` to synchronize your TypeScript definitions with the PostgreSQL schema and generate TypeScript types:

```bash
ddcore migrate
```

Expected output:
```
[INFO] Applying migrations for app 'core'...
[INFO] Applying migrations for app 'library'...
[INFO] Table 'tab_book' created.
[INFO] Generating types under .ddcore/types.d.ts...
[INFO] Migration completed successfully.
```

Your database now contains the `tab_book` table with correct column types, unique constraints, and indexes.

---

## 6. Create an Administrative User

Set the password for the default `Administrator` account:

```bash
ddcore user passwd Administrator admin1234
```

---

## 7. Start the Development Server

Launch the development server with live reload:

```bash
ddcore dev
```

The server listens on `http://localhost:8090`. Open your browser, navigate to the URL, and sign in:
- **User:** `Administrator`
- **Password:** `admin1234`

In the Desk:
1. Open the search bar or navigate to **Library** > **Book**.
2. Click **New Book**.
3. Fill in the fields (`ISBN`, `Title`, `Author`) and click **Save**.

The record is immediately saved to PostgreSQL with audit timestamps (`creation`, `modified`, `owner`).

---

## 8. Add Server-Side Validation

Business logic is written in synchronous TypeScript controllers.

Create `library/doctypes/book/book.controller.ts`:

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

Because `ddcore dev` watches application files, your changes are reloaded automatically in memory without restarting the server.

Try creating a book with `published_year` set to `2099` in the Desk. The server rejects the mutation with an atomic rollback and displays the error message.

---

## 9. Write and Run an Automated Test

Create a unit test in `library/doctypes/book/book.test.ts`:

```typescript
import { ddcore, assert } from "@ddcore/sdk";

export default {
  "creates and validates a book"() {
    const doc = ddcore.newDoc("Book", {
      isbn: "978-0131103627",
      title: "The C Programming Language",
      author: "Brian W. Kernighan, Dennis M. Ritchie",
      published_year: 1978,
      status: "Available",
    });
    doc.insert();

    assert.equal(doc.name, "978-0131103627");
    assert.equal(doc.status, "Available");
  },

  "rejects future publication year"() {
    assert.throws(() => {
      ddcore.newDoc("Book", {
        isbn: "978-9999999999",
        title: "Future Book",
        author: "Time Traveler",
        published_year: 2099,
      }).insert();
    });
  },
};
```

Run tests using the CLI:

```bash
ddcore test --app library
```

Each test runs inside an isolated database transaction that is automatically rolled back after execution, leaving your development database clean.

---

## Next Steps

- Read the [Architecture & Mental Model](/guide/architecture) to understand how `ddcore` handles transactions and code execution.
- Explore the [Example App Walkthrough](/guide/demo) to see relationships, child tables, and workflows.
- Consult the [Server Controller API Reference](/agent/controller-api) for all lifecycle hooks and database methods.
