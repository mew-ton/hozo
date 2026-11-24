# Hozo

English | [日本語](README.ja.md)

**Hozo is a specification language that serves as the single source of truth for an application's design.**

- Requirements and design, even when written by an AI, stay in a form that people can review and correct.
- The reviewed specification becomes the skeleton of the code and its test harness as it is.
- People are responsible for the specification. The bodies can be left to an AI, and the harness checks that they stay within the specification.

Hozo is unreleased and at version 0.x. The language, the IR and the plugin protocol still change without compatibility ([`docs/ir.md`](docs/ir.md), "版").

## What a specification looks like

An excerpt from the sample [`tests/samples/shop-server/server.hozo`](tests/samples/shop-server/server.hozo), with the comments translated:

```
/** Place an order. The order and the record of its notice are stored together. */
@http.post("/orders")
@prd.ref(design.Requirement.PRD_001, design.Requirement.PRD_002)
export fn placeOrder(in: PlaceOrderInput): type.Order {
  val session = authenticate(in.token);
  val order = newOrder(session, in.lines);
  val notice = composeNotice(order);
  store.placeOrder(order, notice);
  mailer.send(notice);
  return order;
}

/** Decide the identifier and the time, and build the order (written by hand) */
fn newOrder(session: type.Session, lines: type.OrderLine[]): type.Order;
```

| In the specification | What `hozoc` does with it |
|---|---|
| A function with a body (`placeOrder`) | The body is wiring: which functions are called, in which order. The code is generated from it. |
| A function without a body (`newOrder`) | A person or an AI writes it. A missing implementation is a compile error in the generated code. |
| A decorator from a vocabulary (`@http.post`) | The plugin that interprets it generates the code: here, the HTTP handler. |
| A link to a requirement (`@prd.ref`) | An annotation: it says which requirements the function serves, and changes no code. Each requirement gets a slot for its tests ([`docs/plugin-go-design.md`](docs/plugin-go-design.md)), and `hozoc tests` checks that the required tests ran. |

The grammar is [`docs/grammar.md`](docs/grammar.md). The guidance on how to write a specification is in [`docs/requirements.md`](docs/requirements.md), "書き方の指針".

## What is in this repository

| Path | Contents |
|---|---|
| `cmd/hozoc`, `internal/` | The compiler `hozoc`: `build` (check and write the code), `tests` (check that the required tests ran), `lsp` (language server) |
| `plugins/` | The plugins. Vocabularies: `http`, `valid`, `db`. Go transpilers: `go`, `go-valid`, `go-http`, `go-sqlite`, `go-cli`, `go-design` |
| `plugin/`, `ir/`, `schema/` | A sample SDK for writing plugins in Go, the IR, and the JSON Schema of the IR |
| `runtime/` | The Go package that the generated code imports |
| `vocab/` | The declarations of the bundled vocabularies |
| `editors/vscode` | The VS Code extension |
| `*.hozo`, `hozo.toml` | Hozo's own specification, written in Hozo. The command line of `hozoc` is generated from `cli.hozo` |
| `tests/` | The corpus, one test per rule, and the samples with hand-written expected output |
| `docs/` | The specification and the design documents |

`hozoc` itself holds no code for any target language: every line of generated code comes from a plugin. A plugin is a WebAssembly module that runs in a sandbox inside `hozoc`, without access to the network or to files ([`docs/plugin-protocol.md`](docs/plugin-protocol.md)).

What does not exist yet: a target language other than Go, and the UI for reviewing and correcting a specification.

## Try it

It needs Go 1.24.7 or later.

```bash
make build
```

```bash
bin/hozoc build --no-emit
```

The second command checks this repository's own specification and writes nothing. `bin/hozoc build --dry-run` lists the files that it would write. The commands are described in [`docs/cli.md`](docs/cli.md).

## Documents

The documents are in Japanese for now. English is the first language of the toolchain, and the compiler's output and the documents will move to it ([`docs/decisions.md`](docs/decisions.md), 442 and 488).

| To know | Read |
|---|---|
| Which document decides what, and the reading order | [`docs/README.md`](docs/README.md) |
| What the language is for | [`docs/requirements.md`](docs/requirements.md), "価値と位置づけ" |
| What has been decided | [`docs/decisions.md`](docs/decisions.md) |
| What is undecided, and what comes next | [`docs/open.md`](docs/open.md) |

## Development

```bash
make test
```

```bash
make generate
```

`make generate` writes `internal/gen/hozo/` again from `cli.hozo`. The checks to run before a push, and what CI checks, are in [`CLAUDE.md`](CLAUDE.md), "確かめ方", and [`docs/testing.md`](docs/testing.md).
