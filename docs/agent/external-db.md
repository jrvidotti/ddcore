# External databases

`ddcore.db.*` reads the site's own Postgres. `ddcore.externalDb(name)` reads **another**
database — a reporting copy of an ERP, say — from a controller, a service, a report or a job.
It is read-only, configured from the environment, and pooled by the framework. Only SQL Server
is supported for now.

```ts
const erp = ddcore.externalDb("sql_server");          // reads DDCORE_SECRET_SQL_SERVER_*
const rows = erp.sql(
  "SELECT TOP 10 CodigoDocumento, ValorSaldo FROM dbo.Documento WHERE CodigoPessoa = @p1",
  [codigoPessoa],
  { timeout: 30 },
);                                                    // Record<string, any>[]
```

Like everything else on the server, the call is synchronous: no `await`.

## Configuration

The credentials live in the environment under the secret prefix, exactly like
`ddcore.secret` (see `auth`), so they never reach a document, a backup or an export. The name
maps to the prefix the same way: `"sql_server"` becomes `DDCORE_SECRET_SQL_SERVER`.

| Variable | Required | Meaning |
|---|---|---|
| `DDCORE_SECRET_<NAME>_HOST` | yes | host name or address |
| `DDCORE_SECRET_<NAME>_PORT` | no | port, default `1433` |
| `DDCORE_SECRET_<NAME>_DATABASE` | yes | database to connect to |
| `DDCORE_SECRET_<NAME>_USER` | yes | login |
| `DDCORE_SECRET_<NAME>_PASSWORD` | yes | password |
| `DDCORE_SECRET_<NAME>_DRIVER` | no | `sqlserver` (the default, and the only one for now) |
| `DDCORE_SECRET_<NAME>_ENCRYPT` | no | go-mssqldb's `encrypt`: `true`, `false`, `strict` or `disable` |

A missing variable is a `ValidationError` naming the variable, never a value:
`External database sql_server: DDCORE_SECRET_SQL_SERVER_USER is not configured`.

## Read-only

- Only a statement that starts with `SELECT` or `WITH` is accepted; anything else is a
  `PermissionError`.
- Each call runs in its own transaction, which is **always rolled back**.
- The connection is opened with `ApplicationIntent=ReadOnly`, which routes to a readable
  secondary where there is one.

None of this is a substitute for the real boundary: **give ddcore a login that can only read**
(`db_datareader`, or `GRANT SELECT` on the schemas it needs). A `WITH` can still hide a
statement with side effects, and only the server's permissions stop it for certain.

## Parameters and timeout

Parameters are positional — `@p1`, `@p2`, … — and are sent to the server as parameters, never
interpolated into the text. Build every value that comes from a user through them.

`opts.timeout` is in seconds; the default is 30. A query that runs past it is cancelled on the
server. A transport or SQL error throws a `ValidationError` that starts with
`External database <name>:`.

## Values

Rows come back as plain objects keyed by column name, converted the way `ddcore.db.sql`
converts Postgres values:

| SQL Server type | JS value |
|---|---|
| `DECIMAL` / `NUMERIC` / `MONEY` / `SMALLMONEY` | number |
| `DATE` | `"YYYY-MM-DD"` |
| `TIME` | `"HH:MM:SS"` |
| `DATETIME` / `DATETIME2` / `SMALLDATETIME` / `DATETIMEOFFSET` | RFC 3339 string |
| `UNIQUEIDENTIFIER` | canonical string, `"6F9619FF-8B86-D011-B42D-00C04FC964FF"` |
| `BIGINT` beyond ±2^53 | string (a JS number would lose digits) |
| `INT`, `BIT`, `NVARCHAR`, … | number, boolean, string |

`DATETIME` and `DATETIME2` carry no offset, so the string is written as UTC. Give every column an
alias: a column without a name comes back under the key `""`.

## Pooling

Each name keeps one small pool (at most 4 open connections), so a busy report cannot flood the
other server. The pool is rebuilt when the variables change, as on a restart with a new `.env`.

## `ddcore doctor`

Every prefix with both a `_HOST` and a `_DATABASE` is listed as an external database, with its
host and database but never its login. Doctor probes each one with `SELECT 1` under a 5-second
timeout. A failure is a warning, not a critical finding: only the code that reads that database
is affected.

## Example: checking a D+1 copy

```ts
// services/erp.ts
export function erpFreshness() {
  const [r] = ddcore.externalDb("sql_server").sql(`
    SELECT @@VERSION AS version, DB_NAME() AS db, SYSDATETIME() AS server_time,
           (SELECT MAX(DataCriacao) FROM dbo.Documento) AS latest_movement`);
  const yesterday = ddcore.utils.addDays(ddcore.utils.nowdate(), -1);
  return { ...r, fresh: String(r.latest_movement).slice(0, 10) >= yesterday };
}
```

## Out of scope

- Writes to the external database.
- Engines other than SQL Server (`_DRIVER` leaves room for them).
- Per-record credentials. Those belong in the vault (see `vault`).
