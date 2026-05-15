# dcbot

A Discord reaction-role bot. Users react to a pinned message to receive roles; removing the reaction revokes the role automatically.

## How it works

On startup the bot posts (or recovers) a single message in the configured role channel. That message lists every emoji → role mapping. Users react to it to get the corresponding role. The Discord message is the source of truth — mappings are re-read from it on every restart.

## Slash commands

Both commands are only visible to members with the **Manage Roles** permission, and must be used in the configured admin channel.

| Command | Description |
|---------|-------------|
| `/addrole <emoji> <role>` | Add an emoji → role mapping |
| `/removerole <emoji>` | Remove an emoji → role mapping |

## Configuration

Create a `.env` file (or set environment variables directly):

```env
# Required
DISCORD_TOKEN=your-bot-token
ROLE_CHANNEL_ID=channel-id-where-the-role-message-is-posted
ADMIN_CHANNEL_ID=channel-id-where-slash-commands-are-used

# Optional — overrides the default header on the role message
ROLE_MESSAGE_HEADER=React below to pick up a role!

# Optional — LLM chat (replies when bot is @-mentioned or its message is replied to)
LLM_ENDPOINT=http://localhost:8000/v1/chat/completions
LLM_MODEL=gemma-pro
LLM_MAX_TOKENS=400
LLM_TEMPERATURE=0.7
LLM_HISTORY_DEPTH=5
LLM_TIMEOUT_SECONDS=30
LLM_SYSTEM_PROMPT_PATH=/path/to/system_prompt.txt   # optional override of built-in persona
```

## Chat

When the bot is `@mentioned` or someone replies to one of its messages, it forwards the message plus the recent context (a reply chain if present, otherwise the previous few channel messages) to an OpenAI-compatible chat completion endpoint (e.g. a local vLLM server) and posts the reply. Slash-command interactions and other bots' messages are filtered out of context. If the LLM is unreachable, the bot replies with `X_X`.

## Running

```bash
go run .
```

## Bot permissions required

When adding the bot to a server, it needs:

- `Read Messages / View Channels`
- `Send Messages`
- `Manage Roles` (must be higher than any role it assigns)
- `Add Reactions`
- `Read Message History`
