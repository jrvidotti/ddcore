import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The email copy of an inbox notification the framework itself writes: a
 * document assigned, a document shared, a task due today.
 *
 * `title` and `message` arrive already written in the reader's language, by
 * whoever recorded the notification; only the button is translated here. The
 * title may quote text somebody typed (a task's description), so the subject
 * takes one line of it and no more.
 */
const oneLine = (s: string) => {
  const line = String(s ?? "").replace(/\s+/g, " ").trim();
  return line.length > 150 ? line.slice(0, 149) + "…" : line;
};

export default defineMailTemplate<{ title: string; message: string; doctype: string; id: string }>({
  name: "core.notification",
  subject: (d) => oneLine(d.title),
  body: (d, b) => [
    b.p(d.message),
    b.button(_("Open document"), ddcore.docUrl(d.doctype, d.id)),
  ],
});
