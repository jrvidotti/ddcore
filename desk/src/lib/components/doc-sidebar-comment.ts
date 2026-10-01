interface CommentRow {
  owner?: string;
  comment_type?: string | null;
  creation?: string;
  modified?: string;
}

/** A person's comment, as opposed to a timeline entry the system wrote in their name. */
const isUserComment = (c: CommentRow) => !c.comment_type || c.comment_type === "Comment";

/** Whether the current user may edit this comment: its author, and nobody else. */
export function canEditComment(c: CommentRow, user: string): boolean {
  return isUserComment(c) && c.owner === user;
}

/** Whether the current user may delete this comment: its author, or a System Manager. */
export function canDeleteComment(c: CommentRow, user: string, isSystemManager: boolean): boolean {
  return isUserComment(c) && (isSystemManager || c.owner === user);
}

/** Whether the comment was changed after it was posted. */
export function isCommentEdited(c: CommentRow): boolean {
  if (!c.creation || !c.modified) return false;
  return new Date(c.modified).getTime() > new Date(c.creation).getTime();
}
