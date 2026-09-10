export async function runDialogAction<T extends { busy: boolean }>(
  action: (values: Record<string, any>, dialog: T) => any,
  values: Record<string, any>,
  dialog: T,
): Promise<void> {
  dialog.busy = true;
  try { await action(values, dialog); } finally { dialog.busy = false; }
}
