import type { DocShare } from "$lib/api";

/** The rights a share grants, strongest last, as English labels to translate. */
export function shareRightLabels(share: Pick<DocShare, "read" | "write" | "share">): string[] {
  const out = ["Can Read"];
  if (share.write) out.push("Can Write");
  if (share.share) out.push("Can Share");
  return out;
}

/** Whether the current user may remove this share: a sharer, or its recipient. */
export function canRemoveShare(share: Pick<DocShare, "user">, canShare: boolean, user: string): boolean {
  return canShare || share.user === user;
}
