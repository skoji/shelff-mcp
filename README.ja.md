# shelff-mcp

AIエージェントで [shelff](https://skoji.dev/shelff/) のPDFライブラリを管理するための MCP (Model Context Protocol) サーバーです。

[English README](./README.md)

## shelff とは

[shelff](https://skoji.dev/shelff/) はiPad用のPDF読書アプリです。PDFごとにサイドカーメタデータ（タイトル、著者、タグ、カテゴリ、読書進捗など）を持ち、iCloud経由で同期されるシンプルなディレクトリツリーで管理します。データベースは不要です。

`shelff-mcp` を使うと、AIエージェントがMCPプロトコル経由でshelffライブラリの閲覧・検索・整理を行えるようになります。

## はじめに

### 1. インストール

**`go install` を使う場合**（Go 1.25以上が必要）:

```bash
go install github.com/skoji/shelff-go/cmd/shelff-mcp@latest
```

**ビルド済みバイナリをダウンロード**: [Releases ページ](https://github.com/skoji/shelff-go/releases)から取得できます。

### 2. AIエージェントの設定

`shelff-mcp` はstdioベースのMCPサーバーです。shelffライブラリの場所を `--root` または環境変数 `SHELFF_ROOT` で指定します。

#### macOS / iCloud

macOSでiCloud同期されたshelffライブラリは、通常以下の場所にあります:

```
$HOME/Library/Mobile Documents/iCloud~jp~skoji~shelff/Documents/
```

パスに空白（`Mobile Documents`）が含まれるため、常にクォートしてください。

#### Claude Desktop

Claude DesktopのMCP設定ファイル（`claude_desktop_config.json`）に追加:

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

#### ChatGPT（MCPプラグイン経由）

ChatGPTのMCP設定に追加:

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

### 3. 使ってみる

設定後、AIエージェントに以下のように話しかけてみてください:

- 「ライブラリの本を一覧して」
- 「どんなタグがある？」
- 「Go_in_Action.pdf に 'programming' タグを追加して」
- 「ライブラリの統計を見せて」

## 注意事項

### ライブラリのバックアップ

`shelff-mcp` はサイドカーメタデータファイルの**作成・変更・削除**が可能です。AIエージェントに大量の変更を行わせる前に、ライブラリのコピーで作業することをお勧めします。

### Claude と大量のスキャン結果

Claudeを使用する場合、ライブラリが大きいと `scan_books` の結果が切り詰められることがあります。`directory`、`limit`、`offset` パラメータを使って結果をページネーションするか、特定のサブディレクトリをスキャンしてください。

### ルートパスの規則

- ルートはshelffライブラリのディレクトリ自体を指す必要があります
- ツールのパスは常にルートからの相対パスです
- 絶対パスは拒否されます
- シンボリックリンクを含む、ルート外へのパストラバーサルは拒否されます

## 利用可能なMCPツール

### 読み取り専用ツール

| ツール | 説明 |
|--------|------|
| `get_specification` | shelffの仕様を取得（概要、サイドカー/カテゴリ/タグスキーマ） |
| `read_metadata` | PDFのメタデータを読み取り（サイドカーがなくても最小メタデータを返す） |
| `scan_books` | ページネーションとディレクトリフィルタ付きで本を一覧 |
| `find_orphaned_sidecars` | 対応するPDFのないサイドカーを検出 |
| `validate_sidecar` | サイドカーをスキーマに対して検証 |
| `library_stats` | ライブラリの統計情報を取得 |
| `collect_all_tags` | ライブラリ全体で使用されているタグを一覧 |
| `read_categories` | カテゴリ定義を読み取り |
| `read_tag_order` | タグの表示順を読み取り |
| `check_library` | ライブラリの診断チェックを実行 |

### 変更ツール

| ツール | 説明 |
|--------|------|
| `create_sidecar` | PDFの新しいサイドカーを作成 |
| `write_metadata` | メタデータを更新（部分マージ、必要に応じてサイドカーを作成） |
| `delete_sidecar` | サイドカーファイルを削除 |
| `move_book` | PDFとサイドカーを別のディレクトリに移動 |
| `rename_book` | PDFとサイドカーをリネーム |
| `add_category` / `remove_category` / `rename_category` / `reorder_categories` | カテゴリ管理 |
| `add_tag_to_order` / `remove_tag_from_order` / `rename_tag` / `reorder_tags` | タグ順序管理 |

`delete_book` はMCP経由では意図的に**公開していません**。エージェントのワークフローによるPDFの誤削除リスクを低減するためです。

## Goライブラリ

基盤となるGoライブラリ（`shelff`）は単独で使用できます。APIの詳細は[ライブラリドキュメント](./docs/library.md)を参照してください。

```bash
go get github.com/skoji/shelff-go/shelff
```

## 関連情報

- [shelff仕様](./shelff-schema/SPECIFICATION.md)
- [shelff iOS/iPadOSアプリ](https://skoji.dev/shelff/)
