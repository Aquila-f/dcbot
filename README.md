# dcbot

A Discord bot built around a small module system. Current modules:

- **Reaction roles** — users react to a pinned message to receive roles; removing the reaction revokes the role.
- **Chat** — when `@mentioned` or replied to, the bot forwards recent context to an OpenAI-compatible LLM endpoint and posts the reply.
- **LeetCode daily** (scheduled) — posts the daily challenge with difficulty, contest rating, and topic tags to a configured channel.

## Architecture

The bot is a thin host (`bot/`) that wires modules together through interfaces declared in `domain/`:

| Interface | Purpose |
|-----------|---------|
| `Module` | Base tag — exposes `Name()` for logs. |
| `CommandProvider` | Module contributes slash commands. |
| `EventSubscriber` | Module attaches gateway event handlers (messages, reactions, members…). |
| `ReadyHook` | Module runs work after the gateway `READY` event (e.g. reconciling state with Discord). |

A module implements whichever subset it needs and is registered in `bot.Bot.Start()`. Existing modules live under `modules/` (one package per module). Recurring jobs run through `scheduler/` and implement the `scheduler.Task` interface.

### Reaction roles

On startup the bot posts (or recovers) a single message in the configured role channel. That message lists every emoji → role mapping. Users react to it to get the corresponding role. The Discord message is the source of truth — mappings are re-read from it on every restart.

### Chat

When the bot is `@mentioned` or someone replies to one of its messages, it forwards the message plus the recent context (a reply chain if present, otherwise the previous few channel messages, plus the thread starter if applicable) to the LLM endpoint and posts the reply. Slash-command interactions and other bots' messages are filtered out of context. If the LLM is unreachable, the bot replies with `X_X`.

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

# Optional — enables the LeetCode daily task; unset to disable
LEETCODE_CHANNEL_ID=channel-id-for-the-daily-post

# Optional — timezone used for cron schedules and the persona's {{TIME}} (default: Asia/Taipei)
TZ=Asia/Taipei

# Optional — LLM chat (replies when bot is @-mentioned or its message is replied to)
LLM_ENDPOINT=http://localhost:8000/v1/chat/completions
LLM_MODEL=gemma-pro
LLM_MAX_TOKENS=400
LLM_TEMPERATURE=0.7
LLM_HISTORY_DEPTH=5
LLM_TIMEOUT_SECONDS=30
LLM_SYSTEM_PROMPT_PATH=/path/to/system_prompt.txt   # optional override of built-in persona
```

## Running

```bash
go run .
```

A smoke utility for the LeetCode task is available at `cmd/leetcode_smoke` (posts once and exits).

## Bot permissions required

When adding the bot to a server, it needs:

- `Read Messages / View Channels`
- `Send Messages`
- `Manage Roles` (must be higher than any role it assigns)
- `Add Reactions`
- `Read Message History`

## Known issues

Tracked here until they get their own fix commits.

- **Scheduler is wired outside the module system.** `bot.Bot.Start()` directly registers `tasks.LeetcodeDaily` on the scheduler, so adding a task means editing the bot. A `domain.ScheduledJobs` interface (or making tasks first-class modules) would make this symmetric with command/event modules.
- **`scheduler/tasks` imports `scheduler`** for the `Payload` type, which gives the `scheduler` ← `tasks` dependency the wrong direction. The shared contract (`Task`, `Payload`) should live in a neutral package both sides depend on.
- **`AppConfig` is becoming a god struct.** Some modules receive a sliced config (`LLMConfig`, `RolesConfig`), others (scheduler) read fields directly off `AppConfig`. Pick one style.
- **`roles.json` path is hardcoded to the working directory.** Changing the cwd creates a fresh store. Should be configurable.
- **Chat reply truncation cuts UTF-8 bytes.** `reply[:2000]` in `modules/chat` can split a multi-byte character. Should truncate on rune boundaries.
- **Role message recovery only inspects the last channel message.** If anything posts in the role channel after the bot's message, recovery fails and a duplicate may be created. Should scan further back for a bot-authored message.
- **Chat handler ignores the bot's root context.** In-flight LLM calls aren't cancelled on shutdown.
- **`scheduler.taskTimeout` is hardcoded to 30s.** Tasks that chain multiple HTTP calls (e.g. LeetCode) should be able to declare their own deadline.
- **`readyErr` channel only handles the first `READY` event.** Reconnects emit additional `READY`s that will block the handler.
- **`checkBotHierarchy` bypasses the state cache.** `s.Guild()` / `s.GuildMember()` round-trip to Discord on every `/addrole`; prefer `s.State.Guild` / `s.State.Member` first.
- **`cmd/leetcode_smoke` requires unrelated env vars.** It calls `config.Load()`, which insists on `ROLE_CHANNEL_ID` and `ADMIN_CHANNEL_ID` even though the smoke tool doesn't use them.
