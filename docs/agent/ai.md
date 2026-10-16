# AI

`ddcore.ai` calls the site's language model from server code: an answer in text, a tool loop, a
structured JSON answer, images in the input, and the transcription of an audio. The provider is an
OpenAI-compatible [Responses API](https://platform.openai.com/docs/api-reference/responses); the
framework's own features (the Feedback dialog's "Fill in with AI", see `feedback`) use the same
one.

```ts
const r = ddcore.ai.respond({
  instructions: "Summarize the customer's message in one sentence, in Portuguese.",
  input: message,
});
r.text; // "O cliente pede a segunda via do boleto."
```

Like everything else on the server, the call is synchronous: no `await`.

## Configuration

The key is a credential, so it lives in the environment only. The models are the site's
decision, in `ddcore.json`, and the environment overrides them where a deployment differs.

```json
{ "ai": { "model": "gpt-6-luna", "transcriptionModel": "gpt-transcribe" } }
```

| Key in `ai` | Environment | Default | Meaning |
|---|---|---|---|
| — | `DDCORE_AI_API_KEY` | — | the provider's key. Without it `ddcore.ai` is off |
| `enabled` | `DDCORE_AI` | on | `false` / `0` turns it off with a key set |
| `baseUrl` | `DDCORE_AI_BASE_URL` | `https://api.openai.com/v1` | the API's root, for another compatible provider or a proxy |
| `model` | `DDCORE_AI_MODEL` | `gpt-6-luna` | answers `respond` when the call names no model |
| `transcriptionModel` | `DDCORE_AI_TRANSCRIPTION_MODEL` | `gpt-transcribe` | answers `transcribe` when the call names no model |
| `reasoningEffort` | `DDCORE_AI_REASONING_EFFORT` | — | sent with every `respond` that names none; unset, none is sent |
| `timeoutSeconds` | `DDCORE_AI_TIMEOUT_SECONDS` | 45 | one `respond`, retries included, when the call names no timeout (0–600) |

A site has one key, for every tenant: the provider bills one account per deployment.
`ddcore.ai.enabled()` says whether a key is set.

## `respond`

```ts
ddcore.ai.respond({
  model?, instructions?, input, tools?, toolChoice?, schema?,
  reasoningEffort?, maxOutputTokens?, includeReasoning?, timeout?,
}): { id, model, status, incompleteReason, output, text, calls, json, usage }
```

- `input` is a string or an array of items: messages (`{ role, content }`, the content a string
  or parts), `function_call_output` items, and items of an earlier answer's `output`. The
  framework sends them as they are.
- The body always carries `store: false`. It also carries `include:
  ["reasoning.encrypted_content"]` unless `includeReasoning: false`, so a reasoning model's
  reasoning goes from one call of a tool loop to the next. A "compatible" provider that refuses
  `include` needs `includeReasoning: false`.
- `text` joins the answer's messages. `calls` are the tools the model asked for (`callId`,
  `name`, `arguments` as JSON text). `usage` is `{ inputTokens, outputTokens }`.
- An answer cut short is `status: "incomplete"` with `incompleteReason` (`max_output_tokens`,
  say). That is an answer, not an error, unless a `schema` was given.

### A tool loop

Replay the answer's `output` untouched, followed by one `function_call_output` per call:

```ts
const input: any[] = [{ role: "user", content: question }];
for (let i = 0; i < 8; i++) {
  const r = ddcore.ai.respond({ instructions, input, tools, timeout: 45 });
  if (!r.calls.length) return r.text;
  input.push(...r.output);
  for (const c of r.calls) {
    input.push({ type: "function_call_output", call_id: c.callId, output: JSON.stringify(run(c.name, JSON.parse(c.arguments))) });
  }
}
```

A loop like this takes seconds per round and holds the request's transaction the whole time, so
run it in a job (`ddcore.enqueue`), not in a whitelisted method a user waits on.

### Structured output

`schema` asks for JSON that matches a JSON Schema (`text.format` of type `json_schema`, strict
by default). The answer comes back parsed in `json`. An answer that is cut short, or is not JSON,
throws. In strict mode every property is listed in `required`, `additionalProperties` is `false`,
and an optional value is a union with `null`:

```ts
const r = ddcore.ai.respond({
  instructions: "Extract the order from the customer's message.",
  input: message,
  schema: { name: "order", schema: {
    type: "object", additionalProperties: false, required: ["sku", "qty", "note"],
    properties: { sku: { type: "string" }, qty: { type: "integer" }, note: { type: ["string", "null"] } },
  } },
});
r.json.qty;
```

### Images

`ddcore.ai.image(base64, contentType, detail?)` makes an `input_image` part for a message's
content:

```ts
ddcore.ai.respond({ input: [{ role: "user", content: [
  { type: "input_text", text: "What is wrong in this screenshot?" },
  ddcore.ai.image(png, "image/png"),
] }] });
```

## `transcribe`

```ts
ddcore.ai.transcribe(audioBase64, { filename, contentType?, model?, language?, prompt?, timeout? }): { text, model }
```

`filename` is required, because the provider reads the format from its extension (`audio.ogg`,
`.mp3`, `.m4a`, `.wav`, `.webm`). The audio is at most 25 MB. `language` is an ISO-639-1 hint
(`"pt"`), and `prompt` steers the spelling of names and jargon. The default timeout is 120
seconds.

## Retries, time, errors

- Each call makes up to 3 attempts. It retries a 429, a 5xx and a dropped connection, and waits
  0.5 s, then 1.5 s, or the provider's `Retry-After` up to 5 s. Any other status fails at once.
- `timeout` (seconds) covers the whole call, retries included. In a job, the job's own timeout
  and its cancellation cut it sooner.
- A refusal of the call itself is a `ValidationError`: no key, no model, an empty input, a 4xx
  from the provider. A failure of the provider is an `UnavailableError`: 5xx, timeout,
  unreachable, `status: "failed"`, invalid JSON for a schema.
- The message carries the provider's own error text, cut to 300 characters
  (`AI request failed: HTTP 400: Invalid file format.`). A provider's raw body is never quoted.

## Privacy

What goes in a prompt leaves the site for the provider. The framework logs one line per attempt
(kind, model, status, latency, attempt) and the token counts. It never logs the prompt, the
answer or the key; do the same in app code.

## Tests

Inside `ddcore test`, `ddcore.ai` never reaches the network. A call throws unless the test set
up `ddcore.test.fakeAi`, so a site's `.env` key is never spent by a test run:

```ts
it("summarizes", () => {
  const ai = ddcore.test.fakeAi({
    respond: (call) => ({ status: "completed", output: [
      { type: "message", content: [{ type: "output_text", text: "Resumo." }] },
    ] }),
    transcribe: (call) => "bom dia",
  });
  expect(summarize("…")).toBe("Resumo.");
  expect(ai.respondCalls[0].body.model).toBe("gpt-6-luna");
});
```

- `respond(call)` receives `{ url, body, timeout }`, where `body` is exactly what the provider
  would get (model defaulted, `store: false`, `text.format`…). It returns the provider's JSON, or
  `{ status, body }` for a raw HTTP answer, so an error can be tested:
  `{ status: 400, body: JSON.stringify({ error: { message: "…" } }) }`.
- `transcribe(call)` receives `{ url, model, filename, contentType, base64, language, prompt,
  timeout }`. It returns the text, or `{ status, body }`.
- `respondCalls` and `transcribeCalls` record every call.
- Handlers left out keep those of an earlier `fakeAi`, and `restore()` puts them back. The fake
  ends with the test.
- `enabled()` and `canTranscribe()` say whether a handler is set.
- Retries are not simulated.
