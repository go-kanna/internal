# internal

[![CI](https://github.com/go-kanna/internal/actions/workflows/ci.yaml/badge.svg)](https://github.com/go-kanna/internal/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/go-kanna/internal/graph/badge.svg)](https://codecov.io/gh/go-kanna/internal)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Shared internals of the [kanna](https://github.com/go-kanna/kanna) code generators.

Go applies the internal rule by import path, so these packages can be imported by any module under
`github.com/go-kanna/` and by nothing else. Nothing here is a public API: versions track whatever the generators need,
and a minor release may break the one before it.

## Packages

- [`cli`](./cli): runs a generator as a command, with the flags, reporting, and exit codes every generator shares.
- [`diag`](./diag): the problems a generator reports, each tied to a source position.
- [`directive`](./directive): finds the `//kanna:<key>` comments that mark a declaration for a generator.
- [`enumtype`](./enumtype): knows the enums kanna-value generates by their shape, and names the functions it generates
  for them, for kanna-value and the generators reading its types alike.
- [`gofile`](./gofile): assembles one generated file, naming its imports and locals so that none collide.
- [`output`](./output): writes, removes, or checks what a run generated, never touching a hand-written file, and
  reads what the destination already declares and requires.
- [`packages`](./packages): loads the packages a generator reads, setting aside its own stale output, reports
  what went wrong as diagnostics, and type-checks them again as they are once what a run writes is in place.
- [`pkgtest`](./pkgtest): builds loaded packages for tests, without putting a module on disk, and compares what a
  generator generates with golden files.
- [`scan`](./scan): reads the struct types a package declares.
- [`suggest`](./suggest): finds the name a misspelled one was probably meant to be.
- [`valuetype`](./valuetype): knows the value types kanna-value generates by their shape, and names their accessor and
  constructors, for kanna-value and the generators reading its types alike.

## Development

```sh
make test
make lint
```

## License

[MIT](./LICENSE)
