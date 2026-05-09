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
```

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
