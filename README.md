# internal

[![CI](https://github.com/go-kanna/internal/actions/workflows/ci.yaml/badge.svg)](https://github.com/go-kanna/internal/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/go-kanna/internal/graph/badge.svg)](https://codecov.io/gh/go-kanna/internal)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Shared internals of the [kanna](https://github.com/go-kanna/kanna) code generators.

Go applies the internal rule by import path, so these packages can be imported by any module under
`github.com/go-kanna/` and by nothing else. Nothing here is a public API: versions track whatever the generators need,
and a minor release may break the one before it.

## Packages

- [`diag`](./diag): the problems a generator reports, each tied to a source position.
- [`packages`](./packages): loads the packages a generator reads, setting aside its own stale output, and reports
  what went wrong as diagnostics.
- [`suggest`](./suggest): finds the name a misspelled one was probably meant to be.

## Development

```sh
make test
make lint
```

## License

[MIT](./LICENSE)
