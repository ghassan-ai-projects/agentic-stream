// Package transport adapts OpenAI-compatible chat-completions endpoints to the
// native executor's model provider port: it builds the HTTP request, bounds the
// client, and decodes JSON and server-sent-event responses. It proposes model
// output and decides nothing.
package transport
