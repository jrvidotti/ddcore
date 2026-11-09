// Where a form's back button (the chevron left of its title) goes.

export interface FormBackInput {
  isSingle: boolean;
  /** the DocType's list, its records under it */
  basePath: string;
  /** the workspace the DocType lives in (`/app` when none owns it) */
  wsPrefix: string;
  /** the list's last URL in this tab, its filters and page in the query */
  remembered?: string;
  /** what the form script set with `frm.setBackRoute` */
  override?: { path: string | null; label?: string };
}

/** The back button's target and its label (undefined: the default "Back"); null hides it. */
export function formBackTarget(input: FormBackInput): { href: string; label?: string } | null {
  const { override } = input;
  if (override) return override.path === null ? null : { href: override.path, label: override.label };
  // a Single has no list: it goes back to its workspace
  if (input.isSingle) return { href: input.wsPrefix };
  const r = input.remembered;
  if (r && (r === input.basePath || r.startsWith(`${input.basePath}?`))) return { href: r };
  return { href: input.basePath };
}
