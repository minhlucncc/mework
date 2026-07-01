---
name: communicator
description: How to format responses for clarity. Use full markdown by default; restrict formatting only when output is known to go to Mezon chat.
---

# Communicator

## Default mode (CLI)

When responding in the CLI, use **full GitHub-flavored markdown**:
- `#` headings for structure
- `[links](url)` for references
- `| tables |` for data
- `> quotes` for context
- `- [ ] task lists` for checklists
- Code blocks with language tags
- Bold, italic, inline code as needed

## Mezon mode (limited markdown)

If the output is known to go to **Mezon chat**, the following applies. Mezon
has limited markdown rendering — follow these rules so messages look good.

### Supported formatting

| Style | How | Example |
|-------|-----|---------|
| **Bold** | `**text**` | **important point** |
| Inline code | `` `code` `` | `mework start --offline` |
| Code block | ```` ```lang ... ``` ```` | Multi-line code |
| Bullet list | `- item` | Lists of things |
| Numbered list | `1. item` | Step-by-step |

### NOT supported in Mezon — avoid

| Feature | Why |
|---------|-----|
| `[links](url)` | Won't render — paste URL as plain text |
| `![images](url)` | Won't render |
| `## headings` | Won't render |
| `| tables |` | Won't render |
| `> quotes` | Won't render |
| `---` rules | Won't render |
| `<html>` tags | Won't render |
| `~~strikethrough~~` | Won't render |
| `- [ ] tasks` | Won't render |

### Style guide (Mezon)

Be warm but concise:

- **Lead with the key message** — don't bury it in context
- Keep responses short — 3-5 paragraphs max
- Use **bold** sparingly for emphasis
- Use bullet lists for multiple items
- Use code blocks for commands, output, or file contents
- Spell URLs as plain text: `https://example.com`
- Paste URLs on their own line so they're tappable in mobile
- Use emoji lightly: ✅ 🎉 ⏳ ❌ 🤔 👋
- When something finishes, celebrate briefly: "All done! 🎉"
- If unsure, ask rather than guess

## General principles

- **Lead with the key message** — don't bury it in context
- Use the formatting that best communicates the information
- For technical answers: code blocks, file paths, and clear structure
- If unsure which mode, default to CLI/full markdown
