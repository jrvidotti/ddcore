# Persistent notifications

An app declares synchronous rules in `notifications/*.notification.ts`. The framework
stores an occurrence for each authorized recipient in the document transaction and
keeps it available while that user is offline. The Desk central is `/app/notifications`.
Run `ddcore migrate` for the additive internal storage and `ddcore types` for SDK declarations.
The development watcher reloads notification files with the app.

## Event rules

```ts
import { defineNotification, _ } from "@ddcore/sdk";

export default defineNotification({
  name: "shop.order_submitted",
  doctype: "Order",
  event: "on_submit",
  condition: (doc, before) => doc.total > 0,
  recipients: (doc, before) => [doc.owner],
  desk: {
    title: (doc) => _("Order {0} submitted", [doc.name]),
    message: (doc) => _("Your order is ready for review."),
  },
  email: {
    template: "shop.order_submitted",
    args: (doc) => ({ order: doc.name }),
  },
});
```

Names are unique across installed apps; prefix them with the app name. Declare one
`event` or one `date` trigger, and at least one of `desk` and `email`. The supported
events are `on_insert`, `on_update`, `on_submit` and `on_cancel`, with the same
lifecycle boundaries as outgoing webhooks. `dbSet`, rename and delete do not trigger
rules. Child DocTypes and internal delivery/audit records cannot be rule targets.
Invalid definitions fail loading, including missing email templates.

`condition`, `recipients`, Desk content and email arguments receive the current
document and the previous document (`null` on insert). All functions are synchronous:
never use `async`, `await` or return a Promise. A false condition skips the occurrence;
evaluation errors roll back the document operation, occurrences and jobs together.

`recipients` returns active User identifiers, not arbitrary external email addresses.
Duplicate recipients collapse to one occurrence. Missing, disabled and unauthorized
users are discarded. Read authorization includes role permissions and the controller's
`hasPermission` and `permissionQuery` hooks. A notification never grants document access.

Desk title and message are plain text. Use literal `_()` keys and translate them in
`translations/<lang>.csv`; content renders in the recipient's language. Email uses an
existing [mail template](mail.md) and the normal delivery record, queue and transport
retries. There is no independent SMTP implementation or external-recipient option.

## Date rules

Replace `event` with a date trigger:

```ts
export default defineNotification({
  name: "shop.order_overdue",
  doctype: "Order",
  date: { field: "due_date", days: 1 },
  condition: (doc) => doc.status === "Open",
  recipients: (doc) => [doc.owner],
  desk: {
    title: (doc) => _("Order {0} is overdue", [doc.name]),
    message: () => _("Review the outstanding order."),
  },
});
```

The field must be `Date` or `Datetime`; `days` is an integer calendar-day offset
(negative before the field value, zero at it, positive after it). The scheduler
queues its internal sweep every five minutes and scans in batches of 100 using the site's timezone, including daylight
saving transitions. It catches up overdue matching documents after downtime and
on first activation, including historical dates. Use `condition` to limit that set.
Date callbacks have no prior save document.

A database uniqueness constraint deduplicates each rule/document/planned-date/recipient
combination, including concurrent scans and process restarts. Once a planned date's
condition has held, the sweep records it and does not evaluate that document for that
date again: recipients are resolved at that moment, and a user added later is not
notified for the same date. A date whose condition does not hold yet is re-evaluated
on every sweep. Changing the due date allows a new occurrence for the new planned date. Event occurrences use an individual
event identity, so separate updates can notify again. Retries do not create new occurrences.

## Reading and authorization

All operations use the authenticated user. There is no recipient selector, even for
an administrator. Every listing, unread count and read-state change rechecks access
to the referenced document, one document read per stored occurrence, so the cost
of a count grows with the user's inbox. Deleted documents and revoked access hide the occurrence;
renames update its document reference. Email attempts recheck access before sending.
An email already sent cannot be withdrawn after access changes.

The internal `ddcore_notification` table is not a DocType. Generic REST, reports and
export cannot expose it. Use these dedicated endpoints:

| Operation | Request | Response |
| --- | --- | --- |
| List | `GET /api/notifications?limit=20&offset=0&read=false` | `{data: {data: Notification[], total: number}}` |
| Unread count | `GET /api/notifications/count` | `{data: number}` |
| Set read state | `PATCH /api/notifications/{name}` with `{read: true}` or `{read: false}` | `{data: Notification}` |

`limit` defaults to 20 and accepts 1–100; `offset` defaults to zero and is nonnegative.
Omit `read` for both states, or use `true` or `false`. Results are newest first and
the total reflects the filter and current access. Unknown or repeated query parameters
are rejected. A notification has `name`, `title`, `message`, `creation`, `read`,
`reference_doctype` and `reference_name`.

The Desk SDK exposes `ddcore.notifications.list({limit, offset, read})`,
`ddcore.notifications.count()` and `ddcore.notifications.setRead(name, read)`. The central
shows unread count, document links and individual read/unread actions. It refreshes
on opening, tab return, SSE updates and reconnection, so persisted notifications
produced offline are recovered. Logging out clears the local connection and state.

`notifications_changed` is an after-commit SSE invalidation sent only to the affected
recipient. It carries no document content; clients fetch the authorized persisted
state. Rollback emits no invalidation. SSE is a refresh hint, not the durable inbox.

This version does not include a visual rule editor, user preferences, push delivery,
assignments or custom event triggers.
