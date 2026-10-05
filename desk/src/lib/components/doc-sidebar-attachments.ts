// The files attached to a document as a whole (not through one of its Attach
// fields, which show their own file), listed in the form's sidebar; an audio
// one gets a player, so a voice note is heard where it was left.

export interface AttachedFile {
  id: string;
  file_name?: string | null;
  file_url: string;
  file_size?: number | null;
  attached_to_field?: string | null;
}

/** The extensions a browser plays in an <audio> element (a voice note is .webm or .m4a). */
export const AUDIO_EXTENSIONS = ["webm", "ogg", "oga", "m4a", "mp3"];

export function isAudioFile(name: unknown): boolean {
  const m = /\.([a-z0-9]+)$/i.exec(String(name ?? "").split(/[?#]/)[0]);
  return !!m && AUDIO_EXTENSIONS.includes(m[1].toLowerCase());
}

/** The document's own attachments: a file behind an Attach field is shown by that field. */
export function looseAttachments<T extends AttachedFile>(rows: T[] | null | undefined): T[] {
  return (rows ?? []).filter((r) => r && r.file_url && !r.attached_to_field);
}

/** The DocTypes whose files arrive with no Attach field to show them: a Feedback's. */
export function hasLooseAttachments(doctype: string): boolean {
  return doctype === "Feedback";
}

/** The list request for a document's files. */
export function attachmentsQuery(doctype: string, id: string) {
  return {
    filters: [["attached_to_doctype", "=", doctype], ["attached_to_id", "=", id]],
    fields: ["id", "file_name", "file_url", "file_size", "attached_to_field"],
    order_by: "creation asc",
    limit: 50,
  };
}
