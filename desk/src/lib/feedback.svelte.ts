// Whether the Feedback modal is open, and what it opens with: the user menu
// opens it blank, an app (ddcore.ui.openFeedback) may pick the type and title.
import type { FeedbackType } from "./api";

export interface FeedbackPreset {
  type?: FeedbackType;
  title?: string;
}

export const feedbackState = $state<{ open: boolean; preset: FeedbackPreset | null; seq: number }>({ open: false, preset: null, seq: 0 });

export function openFeedback(preset?: FeedbackPreset) {
  feedbackState.preset = preset ? { type: preset.type, title: preset.title } : null;
  // a new opening applies its preset even when the modal is already open
  feedbackState.seq++;
  feedbackState.open = true;
}

export function closeFeedback() {
  feedbackState.open = false;
}
