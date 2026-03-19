# git-ac 🤖

AI-powered commit message generator. Reads your `git diff`, generates a conventional commit message, and commits — all in one command.

Supports **OpenAI**, **Claude**, **Gemini**, and **Ollama** (local). Zero dependencies.

Available in **Go** (single binary) and **Node.js** — pick your flavor.

## Install

### Go

```bash
go install github.com/artty335/commitly@latest
```

Or download a prebuilt binary from [Releases](https://github.com/artty335/commitly/releases).

### Node.js

```bash
npx git-ac
# or install globally
npm install -g git-ac
```

### Build from source

```bash
git clone https://github.com/artty335/commitly.git
cd commitly

# Go
go build -o git-ac .

# Node.js — just run directly
node git-ac.mjs
```

## Setup

Set one of these environment variables:

| Provider | Env Variable | Default Model |
|----------|-------------|---------------|
| Claude | `ANTHROPIC_API_KEY` | `claude-sonnet-4-6-20250514` |
| OpenAI | `OPENAI_API_KEY` | `gpt-4o-mini` |
| Gemini | `GEMINI_API_KEY` | `gemini-2.5-flash` |
| Ollama | (none needed) | `llama3.2` |

The provider is auto-detected from your env vars. Claude is preferred if both keys are set.

## Usage

```bash
# Stage your changes, then:
git-ac

# Auto-commit without confirmation
git-ac -y

# Choose a provider
git-ac -p ollama
git-ac -p openai
git-ac -p gemini

# Use a specific model
git-ac -p claude -m claude-haiku-4-5-20251001

# Edit the message before committing
# → type 'e' at the prompt
```

### As a git alias

Add to your `~/.gitconfig`:

```ini
[alias]
    ac = !git-ac
```

Now you can just run:

```bash
git ac
```

## How it works

1. Reads staged diff (`git diff --cached`), or unstaged diff if nothing is staged
2. Sends the diff to your chosen AI provider
3. Shows the generated commit message
4. You confirm (Y), edit (e), or abort (n)
5. Commits with the message

## Example

```
$ git ac
🤖 Generating commit message with claude...

──────────────────────────────────────────────────
feat(auth): add JWT token refresh on expiry

- Add refresh token rotation in auth middleware
- Update token storage to handle new refresh flow
──────────────────────────────────────────────────

Commit with this message? [Y/n/e(dit)] y
[main abc1234] feat(auth): add JWT token refresh on expiry
 2 files changed, 45 insertions(+), 12 deletions(-)
```

## License

MIT
