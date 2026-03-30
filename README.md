# shelff-mcp

An MCP (Model Context Protocol) server for managing [shelff](https://skoji.dev/en/shelff/) PDF libraries with AI agents.

[日本語版 README](./README.ja.md)

## What is shelff?

[shelff](https://skoji.dev/en/shelff/) is a PDF reading app for iPad. It manages a library of PDFs with sidecar metadata (title, author, tags, categories, reading progress, etc.), organized as a simple directory tree synced via iCloud — no database required.

`shelff-mcp` lets AI agents read, search, and organize your shelff library through the MCP protocol.

## Getting started

### 1. Install

**Using `go install`** (requires Go 1.25+):

```bash
go install github.com/skoji/shelff-go/cmd/shelff-mcp@latest
```

**Download a prebuilt binary** from the [Releases page](https://github.com/skoji/shelff-go/releases).

### 2. Configure your AI agent

`shelff-mcp` is a stdio-based MCP server. You need to tell it where your shelff library lives, using `--root` or the `SHELFF_ROOT` environment variable.

#### macOS / iCloud

On macOS, a shelff library synced via iCloud typically lives at:

```
$HOME/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/
```

Because the path contains a space (`Mobile Documents`), always quote it.

#### Claude Desktop

Add to your Claude Desktop MCP config (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "shelff": {
      "command": "shelff-mcp",
      "args": ["--root", "/Users/<user>/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/"]
    }
  }
}
```

#### Claude Code

```bash
claude mcp add shelff -- shelff-mcp --root "$HOME/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/"
```

#### ChatGPT (via MCP plugin)

Add to your ChatGPT MCP config:

```json
{
  "mcpServers": {
    "shelff": {
      "command": "shelff-mcp",
      "args": ["--root", "/Users/<user>/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/"]
    }
  }
}
```

### 3. Start using it

Once configured, ask your AI agent things like:

- "Show me all books in my library"
- "What tags do I have?"
- "Add the tag 'programming' to Go_in_Action.pdf"
- "What are my library statistics?"

## Important notes

### Back up your library

`shelff-mcp` can **create, modify, and delete** sidecar metadata files in your library. Before letting an AI agent make bulk changes, consider working on a copy of your library first.

### Claude and large scan results

When using Claude, `scan_books` results may be truncated if your library is large. Use the `directory`, `limit`, and `offset` parameters to paginate through results, or scan specific subdirectories.

### Root path rules

- The root must point at the shelff library directory itself
- Tool paths are always relative to that root
- Absolute paths are rejected
- Path traversal outside the root is rejected, including symlink escapes

## Available MCP tools

### Read-only tools

| Tool | Description |
|------|-------------|
| `get_specification` | Retrieve the shelff spec (overview, sidecar/categories/tags schema) |
| `read_metadata` | Read metadata for a PDF (returns minimal metadata even without sidecar) |
| `scan_books` | List books with pagination and directory filtering |
| `find_orphaned_sidecars` | Find sidecar files with no matching PDF |
| `validate_sidecar` | Validate a sidecar against the schema |
| `library_stats` | Get library statistics |
| `collect_all_tags` | List all tags used across the library |
| `read_categories` | Read category definitions |
| `read_tag_order` | Read tag display order |
| `check_library` | Run diagnostic checks on the library |

### Mutation tools

| Tool | Description |
|------|-------------|
| `create_sidecar` | Create a new sidecar for a PDF |
| `write_metadata` | Update metadata (partial merge, creates sidecar if needed) |
| `delete_sidecar` | Delete a sidecar file |
| `move_book` | Move a PDF and its sidecar to another directory |
| `rename_book` | Rename a PDF and its sidecar |
| `add_category` / `remove_category` / `rename_category` / `reorder_categories` | Manage categories |
| `add_tag_to_order` / `remove_tag_from_order` / `rename_tag` / `reorder_tags` | Manage tag order |

`delete_book` is intentionally **not** exposed via MCP, to reduce the risk of destructive PDF deletion from agent workflows.

## Go library

The underlying Go library (`shelff`) can be used independently. See the [library documentation](./docs/library.md) for API details.

```bash
go get github.com/skoji/shelff-go/shelff
```

## See also

- [shelff specification](./shelff-schema/SPECIFICATION.md)
- [shelff iOS/iPadOS app](https://skoji.dev/en/shelff/)
