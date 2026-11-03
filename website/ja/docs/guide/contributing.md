# コントリビュート

Gately はコミュニティ主導のオープンソースプロジェクトで、Issue と Pull Request を歓迎します。リポジトリ：<https://github.com/wangwanjie/gately>

## ローカル開発

必要なもの：Go 1.26+、Node.js 24+、pnpm。

```bash
make web      # フロントエンドをビルド（web/dist に出力され、go:embed で埋め込まれます）
make build    # bin/gately をコンパイル
make test     # go test -race ./...
make lint     # gofmt、go vet、フロントエンドの型チェック
```

フロントエンドの開発時は `cd web && pnpm dev` を実行します。`/api` はローカルの `127.0.0.1:16680` にプロキシされます。

## コントリビュートの決まりごと

- 互換性のためのコードは、公開されている観察可能な挙動、インターフェース、設定形式のみに基づいてください。サードパーティ製ソフトウェアのバイナリやフロントエンド成果物から複製したコードは提出しないでください。
- 新しいモジュールには、設定構造とバリデーション、実行時の起動・停止、REST エンドポイント、ユニットテストが必要です。設定のインポートに対応するモジュールは `luckyimport.Importer` を実装し、概要ページに状態を表示したいモジュールは `module.Summarizer` を実装します。
- オプションのモジュールは `internal/modules/registry/mod_<モジュール>.go` で登録し、ファイルに `//go:build !no_<モジュール>` を付けたうえで、`catalog.go` とルートの `modules.json` にも登録します（`TestManifestMatchesCode` が 3 つの一致を確認します）。モジュール間の連携（サイトの WAF、証明書の SSH や Docker へのデプロイなど）は注入された関数を通じて提供し、相手のモジュールがない場合は「このビルドに含まれていない」と返します。コアのコードからオプションのモジュールのパッケージを直接参照してはいけません。CI は 3 つのプリセットのタグの組み合わせで vet とコンパイルを実行します。
- コードのコメントは中国語で書き、制御文には必ず波括弧を使います。

## 追加のテスト

一部のテストはデフォルトでスキップされ、追加の条件が必要です。

| テスト | 有効化の方法 |
| --- | --- |
| ACME エンドツーエンド（Pebble） | pebble と pebble-challtestsrv をインストールし、`GATELY_PEBBLE_BIN`、`GATELY_PEBBLE_SRC` を設定する |
| 実際の公開 STUN サーバー | `GATELY_NET_TEST=1` |
| 実際の Lucky 設定でのインポート検証 | `GATELY_LUCKY_SAMPLE` など。各モジュールの `TestImportRealSample` を参照 |

## ライセンス

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## 翻訳

- 管理画面の文言は `web/src/locales/<language>/<namespace>.json` にあります。原文の言語は簡体字中国語です。文言を追加・変更するときは少なくとも `zh-CN` と `en` を更新し、提出前に `cd web && node scripts/check-i18n.mjs` を実行してください。規約は `web/src/locales/README.md` を参照してください。
- Web サイトの言語パックは `website/.vitepress/i18n/<language>.json` に、各言語のページは `website/<language dir>/` にあります。
- バックエンドの実行時の文言（エラーメッセージ、ログ、通知、API ドキュメント）は、ソースコード内の中国語の文字列リテラルをキーとし、翻訳は `internal/i18n/locales/<language>.json` にあります。中国語の文言を追加・変更したら `go run ./internal/i18n/cmd/i18nsync -root .` を実行して新しいキーを各言語のファイルに同期し、翻訳を埋めてください。`go test ./internal/i18n` は、すべての言語がそろっているか、書式指定子が原文と一致しているかを検査します。値を埋め込む文は、中国語の断片をいくつもつなぎ合わせず、文全体を 1 つの `fmt.Sprintf` 書式文字列にしてください。そうすれば各言語が独自の語順に従えます。
- API ドキュメントはプログラムが言語ごとに生成します。`make api-docs` が `docs/api/<language>/` に出力するため、手作業での同期は不要です。
- 対応言語：简体中文、English、繁體中文、日本語、한국어、Español、Français、Deutsch、Русский、Português (Brasil)。
