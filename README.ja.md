# internal

[![CI](https://github.com/go-kanna/internal/actions/workflows/ci.yaml/badge.svg)](https://github.com/go-kanna/internal/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/go-kanna/internal/graph/badge.svg)](https://codecov.io/gh/go-kanna/internal)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![English](https://img.shields.io/badge/lang-English-blue.svg)](README.md)

[kanna](https://go-kanna.github.io/ja/) コードジェネレータが共有する内部パッケージです。

Go の internal ルールはインポートパスで適用されるので、これらのパッケージは `github.com/go-kanna/` 配下のモジュールからしかインポートできません。ここに公開 API はありません。バージョンはジェネレータの必要に応じて上がり、マイナーリリースで前のバージョンとの互換性がなくなることもあります。

## パッケージ

- [`cli`](./cli): ジェネレータをコマンドとして実行します。全ジェネレータに共通のフラグ、レポート、終了コードを扱います。
- [`diag`](./diag): ジェネレータが報告する問題を、ソースの位置と結び付けて表します。
- [`directive`](./directive): 宣言にジェネレータ用の印を付ける `//kanna:<key>` コメントを見つけます。
- [`enumtype`](./enumtype): kanna-value が生成する enum を形で見分け、生成される関数の名前を決めます。kanna-value 自身と、その型を読み取るジェネレータの両方が使います。
- [`gofile`](./gofile): 生成ファイルを 1 つ組み立て、インポートとローカル変数の名前が衝突しないように付けます。
- [`output`](./output): 実行で生成したものの書き出し、削除、チェックを行います。手書きのファイルには決して触れません。出力先がすでに宣言しているものと必要としているものも読み取ります。
- [`packages`](./packages): ジェネレータが読むパッケージを、自身の古い出力を除いて読み込みます。問題は診断として報告し、書き出す内容を組み込んだ状態でもう一度型チェックします。
- [`pkgtest`](./pkgtest): テスト用に、ディスク上にモジュールを置かずにパッケージを読み込み、ジェネレータの生成結果をゴールデンファイルと比較します。
- [`scan`](./scan): パッケージが宣言する型と、構造体のフィールドを読み取ります。
- [`suggest`](./suggest): 打ち間違えた名前から、意図されたと思われる名前を見つけます。
- [`valuetype`](./valuetype): kanna-value が生成する値オブジェクトを形で見分け、アクセサとコンストラクタの名前を決めます。kanna-value 自身と、その型を読み取るジェネレータの両方が使います。

## 開発

```sh
make test
make lint
```

## ライセンス

[MIT](./LICENSE)
