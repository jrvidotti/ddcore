import { describe, expect, it, vi } from "vitest";
import {
  AUDIO_MIME_CANDIDATES, MAX_RECORDING_SECONDS, audioExtension, formatElapsed, isPermissionDenied,
  pickAudioMime, recordingAvailable, stopStream, voiceNoteName,
} from "./audio-recorder-state";

describe("recordingAvailable", () => {
  const win = (over: Record<string, any> = {}) => ({
    isSecureContext: true,
    navigator: { mediaDevices: { getUserMedia: () => {} } },
    MediaRecorder: function () {},
    ...over,
  });

  it("needs a secure context, getUserMedia and MediaRecorder", () => {
    expect(recordingAvailable(win())).toBe(true);
    expect(recordingAvailable(win({ isSecureContext: false }))).toBe(false);
    expect(recordingAvailable(win({ navigator: {} }))).toBe(false);
    expect(recordingAvailable(win({ MediaRecorder: undefined }))).toBe(false);
    expect(recordingAvailable(undefined)).toBe(false);
  });
});

describe("pickAudioMime", () => {
  it("takes the first format the browser records", () => {
    expect(pickAudioMime({ isTypeSupported: () => true })).toBe("audio/webm;codecs=opus");
    expect(pickAudioMime({ isTypeSupported: (t) => t === "audio/mp4" })).toBe("audio/mp4");
    expect(pickAudioMime({ isTypeSupported: (t) => t === "audio/ogg" })).toBe("audio/ogg");
  });

  it("lets MediaRecorder choose when it cannot say", () => {
    expect(pickAudioMime({ isTypeSupported: () => false })).toBe("");
    expect(pickAudioMime({})).toBe("");
    expect(pickAudioMime(undefined)).toBe("");
    expect(pickAudioMime({ isTypeSupported: () => { throw new Error("x"); } })).toBe("");
  });

  it("tries WebM/Opus first", () => {
    expect(AUDIO_MIME_CANDIDATES[0]).toBe("audio/webm;codecs=opus");
  });
});

describe("file names", () => {
  it("follows the recorded format", () => {
    expect(audioExtension("audio/webm;codecs=opus")).toBe("webm");
    expect(audioExtension("audio/mp4")).toBe("m4a");
    expect(audioExtension("audio/ogg; codecs=opus")).toBe("ogg");
    expect(audioExtension("")).toBe("webm");
    expect(voiceNoteName("audio/mp4")).toBe("voice-note.m4a");
    expect(voiceNoteName("audio/webm")).toBe("voice-note.webm");
  });
});

describe("formatElapsed", () => {
  it("shows mm:ss", () => {
    expect(formatElapsed(0)).toBe("00:00");
    expect(formatElapsed(5.9)).toBe("00:05");
    expect(formatElapsed(65)).toBe("01:05");
    expect(formatElapsed(MAX_RECORDING_SECONDS)).toBe("05:00");
    expect(formatElapsed(-3)).toBe("00:00");
  });
});

describe("isPermissionDenied", () => {
  it("knows a refused microphone", () => {
    expect(isPermissionDenied({ name: "NotAllowedError" })).toBe(true);
    expect(isPermissionDenied({ name: "NotFoundError" })).toBe(false);
    expect(isPermissionDenied(null)).toBe(false);
  });
});

describe("stopStream", () => {
  it("stops every track", () => {
    const a = { stop: vi.fn() }, b = { stop: vi.fn(() => { throw new Error("ended"); }) };
    stopStream({ getTracks: () => [a, b] });
    expect(a.stop).toHaveBeenCalledOnce();
    expect(b.stop).toHaveBeenCalledOnce();
    expect(() => stopStream(null)).not.toThrow();
  });
});
