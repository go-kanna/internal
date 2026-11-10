# internal

[![CI](https://github.com/go-kanna/internal/actions/workflows/ci.yaml/badge.svg)](https://github.com/go-kanna/internal/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/go-kanna/internal/graph/badge.svg)](https://codecov.io/gh/go-kanna/internal)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![English](https://img.shields.io/badge/lang-English-blue.svg)](README.md)

[kanna](https://go-kanna.github.io/ja/) コードジェネレータが共有する内部パッケージです。

Go の internal ルールはインポートパスで適用されるので、これらのパッケージは `github.com/go-kanna/` 配下のモジュールからしかインポートできません。ここに公開 API はありません。バージョンはジェネレータの必要に応じて上がり、マイナーリリースで前のバージョンとの互換性がなくなることもあります。

ただし、利用者のモジュールでは、tool にしたすべてのジェネレータに internal の版が 1 つだけ選ばれます（どれかが要求するうちで最も新しい版）。公開済みのジェネレータは、公開したときより新しい internal でビルドされ、生成するものもその版に従うことがあります。公開済みのジェネレータが呼ぶものを壊す変更は、利用者がそのジェネレータを上げるまで、利用者のモジュールでのコンパイルを止めます。そのため CI は、ここへの変更ごとに、すべてのジェネレータと kanna の最新のリリースをビルドします。

## パッケージ

- [`cli`](./cli): ジェネレータをコマンドとして実行します。全ジェネレータに共通のフラグ、レポート、終了コードを扱います。
- [`deps`](./deps): ジェネレータが実行の前に `kanna` コマンドへ伝える内容と、コマンドが実行するジェネレータへ渡す内容です。
- [`diag`](./diag): ジェネレータが報告する問題を、ソースの位置と結び付けて表します。
- [`directive`](./directive): 宣言にジェネレータ用の印を付ける `//kanna:<key>` コメントを見つけます。
- [`enumtype`](./enumtype): kanna-value が生成する enum を形で見分け、生成される関数の名前を決めます。kanna-value 自身と、その型を読み取るジェネレータの両方が使います。
- [`gofile`](./gofile): 生成ファイルを 1 つ組み立て、インポートとローカル変数の名前が衝突しないように付けます。
- [`output`](./output): 実行で生成したものの書き出し、削除、チェックを行います。手書きのファイルには触れません。出力先のパッケージがすでに宣言している名前と、go.mod が require しているモジュールも読み取ります。
- [`packages`](./packages): ジェネレータが読むパッケージを、自身の古い出力を除いて読み込みます。問題は診断として報告し、書き出す内容を組み込んだ状態でもう一度型チェックします。
- [`pkgtest`](./pkgtest): テスト用に、ディスク上にモジュールを置かずにパッケージを読み込み、ジェネレータの生成結果をゴールデンファイルと比較します。
- [`qualifier`](./qualifier): タグやディレクティブの中の名前（`rule=model.CompletedAt` の `model`）がどのパッケージを指すかを、Go がコードでするのと同じく、書かれたファイルの import で決めます。
- [`scan`](./scan): パッケージが宣言する型と、構造体のフィールドを読み取ります。
- [`suggest`](./suggest): 打ち間違えた名前から、意図されたと思われる名前を見つけます。
- [`validation`](./validation): kanna-validator が validate タグをどう読み、生成するコードがそのタグで値をどう確かめるかを定めます。kanna-validator 自身と、タグで構造体を確かめるほかのジェネレータの両方が使います。
- [`valuetype`](./valuetype): kanna-value が生成する値オブジェクトを形で見分け、アクセサとコンストラクタの名前を決めます。kanna-value 自身と、その型を読み取るジェネレータの両方が使います。

## 開発

```sh
make test
make lint
```

## ライセンス

[MIT](./LICENSE)
