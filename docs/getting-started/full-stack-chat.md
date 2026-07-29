# Full-stack chat

Build a React chat that sends conversation history to a Go endpoint and renders
the response as it streams. The Go SDK writes the protocol consumed by
`@ai-sdk/react`, so the client and server share the same message format.

## Before you start

You need:

- a working model from [Installation](installation.md);
- Node.js and npm for the React application;
- an `ANTHROPIC_API_KEY` for the runnable backend example.

A completed Go agent backend that extends this handler with a typed tool lives
in [`examples/agent-chat`](../../examples/agent-chat).

## Run the Go backend

The backend accepts `UIMessage` history, starts a model stream, and writes the
assistant message as Server-Sent Events:

```go
mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []aisdk.UIMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithSystem("You are a helpful assistant."),
		aisdk.WithMessages(body.Messages...),
	)
	if err := aisdk.WriteUIMessageStream(
		w,
		result,
		aisdk.WithUIMessageStreamReasoning(false),
	); err != nil {
		log.Printf("streaming chat response: %v", err)
	}
})
```

`WithMessages` converts the frontend history into provider messages. The HTTP
helper sets the streaming headers, flushes chunks, and writes the terminating
`[DONE]` event. The example keeps model reasoning on the server; expose it only
when the application intends to show or process it in the browser.

From a repository checkout, start the complete backend:

```bash
cd examples/agent-chat
ANTHROPIC_API_KEY=sk-... go run .
```

It listens on `http://localhost:8080`.

## Create the React application

In another terminal, create a React application and install the AI SDK frontend
packages:

```bash
npm create vite@latest ai-chat-web -- --template react-ts
cd ai-chat-web
npm install
npm install ai@7.0.37 @ai-sdk/react@4.0.40
```

Replace `src/App.tsx` with:

```tsx
import { useChat } from "@ai-sdk/react";
import { DefaultChatTransport } from "ai";
import { useState } from "react";

export default function App() {
  const [input, setInput] = useState("");
  const { messages, sendMessage, error } = useChat({
    transport: new DefaultChatTransport({ api: "/api/chat" }),
  });

  return (
    <main>
      {messages.map((message) => (
        <div key={message.id}>
          <strong>{message.role}</strong>
          {message.parts.map((part, index) =>
            part.type === "text" ? <p key={index}>{part.text}</p> : null,
          )}
        </div>
      ))}

      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!input.trim()) return;
          sendMessage({ text: input });
          setInput("");
        }}
      >
        <input
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder="Ask a question"
        />
        <button type="submit">Send</button>
      </form>

      {error ? <p>Unable to complete the response.</p> : null}
    </main>
  );
}
```

During local development, proxy `/api` to the Go server. Replace
`vite.config.ts` with:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
```

Start the frontend:

```bash
npm run dev
```

Open the URL printed by Vite and send a message. Vite forwards `/api/chat` to
the Go server while preserving the streaming response.

In production, serve the frontend and API from the same origin or configure the
reverse proxy and CORS policy explicitly. Pass `r.Context()` to model calls so a
browser disconnect cancels the provider request.

## Add application behavior

- Add server-side actions with [typed tools](../guides/tools.md).
- Require confirmation for risky actions with [tool approval](../guides/tool-approval.md).
- Reuse configured model and tool settings with [agent loops](../guides/agent-loops.md).
- Persist conversation history with [Messages](../concepts/messages.md).

The minimal backend accepts any request body so the first-run path stays focused.
Before deployment, authenticate the caller and limit the request body before
JSON decoding. Apply the [production checklist](../best-practices/production.md)
and [error-handling guidance](../best-practices/error-handling.md).

---

← [Generate text from Go](backend-only.md) · [Docs index](../README.md) · [Structured output →](../guides/structured-output.md)
