// Package langfuse provides Langfuse observability integration for new-api.
//
// Langfuse (https://langfuse.com) is an open-source LLM observability platform
// that enables tracing, evaluation, and monitoring of LLM applications.
//
// Configuration is done via environment variables:
//
//	LANGFUSE_HOST       - Langfuse server URL (e.g. https://cloud.langfuse.com)
//	LANGFUSE_PUBLIC_KEY - Your Langfuse project public key
//	LANGFUSE_SECRET_KEY - Your Langfuse project secret key
//
// Usage in new-api:
//
//  1. Register the Gin middleware on the relay router:
//     router.Use(langfuse.Middleware())
//
//  2. In the relay handler, after obtaining the LLM response, call:
//     langfuse.TrackGeneration(langfuse.GenerationParams{...})
package langfuse
