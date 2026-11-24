# Hozo

English | [日本語](README.ja.md)

**Hozo is a specification language that serves as the single source of truth for an application's design.**

When an AI writes most of the code, what people still have to own is the design: what the application does, in which order, and what it must never do. Hozo keeps that design in a form that people can review and correct, and turns the reviewed design into the skeleton of the code and its test harness as it is. People are responsible for the specification. The bodies can be left to an AI.

## What you write, and what you get

You write a specification. This is an abridged excerpt from the sample [`tests/samples/shop-server/server.hozo`](tests/samples/shop-server/server.hozo):

```
/** Place an order. The order and the record of its notice are stored together. */
@http.post("/orders")
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

`hozoc build` writes the code. This is an abridged excerpt from the Go that the sample expects, [`tests/samples/shop-server/gen/server`](tests/samples/shop-server/gen/server):

```go
// inner/wiring.go: a function with a body is wiring. Its code is generated.
func (o *ops) PlaceOrder(ctx context.Context, in PlaceOrderInput) (pkgtype.Order, error) {
	session, err := o.Authenticate(ctx, in.Token)
	// …
	order, err := o.NewOrder(ctx, session, in.Lines)
	// …
}

// inner/ops.go: a function without a body is yours to write.
// A value that lacks even one method does not compile where it is passed.
type Leaves interface {
	NewOrder(ctx context.Context, session pkgtype.Session, lines []pkgtype.OrderLine) (pkgtype.Order, error)
	// …
}

// servertest/leaves.go: the test cases that each of those functions needs.
// A value that lacks even one case does not compile either.
type LeafCases interface {
	NewOrder() NewOrderCases
	// …
}

// http.go: the decorator @http.post is interpreted by a plugin.
func NewHandler(ops Ops) http.Handler
```

The comments in both excerpts are translated or added here. The compiler's own output is in Japanese today.

## Who guarantees what

| What | Who makes sure of it |
|---|---|
| The order of the calls, and what is passed to what | Hozo: the code is generated from the specification |
| Every function without a body has an implementation | The compiler of the target language (`Leaves` in the excerpt) |
| Every test case that the specification requires is written | The compiler of the target language (`LeafCases` in the excerpt) |
| The required tests ran | `hozoc tests --coverage` |
| The tests passed | Your own test run |
| The body of a function, and the contents of a test case | The person or the AI who writes them |

Two things Hozo leaves out on purpose:

- **A specification has no conditions.** It says in which case what happens, and not how the case is told. That is a body.
- **A plugin does not infer its output** from the way you write or from the types. What is generated is decided by declarations and decorators.

The reasons are in [`docs/requirements.md`](docs/requirements.md), "価値と位置づけ" and "出力を推論しない".

## Status

Hozo is unreleased and at version 0.x. The language, the IR and the plugin protocol still change without compatibility.

| | |
|---|---|
| Works today | `hozoc build` (check and write the code), `hozoc tests` (check that the required tests ran), `hozoc lsp` (language server), the VS Code extension |
| Target language | Go only: types and functions, validation, `net/http` handlers, SQLite, command lines |
| Plugins | Every line of generated code comes from a plugin. A plugin is a WebAssembly module that runs in a sandbox inside `hozoc`, without access to the network or to files |
| Not yet | Other target languages. The UI for reviewing and correcting a specification. A license |

## Try it

It needs Go 1.24.7 or later.

```bash
make build
```

```bash
bin/hozoc build --no-emit
```

The second command checks this repository's own specification (`hozoc` is specified in Hozo: `cli.hozo` and the other `*.hozo` files at the root) and writes nothing. `bin/hozoc build --dry-run` lists the files that it would write.

To read a complete pair of a specification and its code, open [`tests/samples/shop-server`](tests/samples/shop-server): the `*.hozo` files are the specification, `gen/` is the generated code, and `app/` is the code written by hand.

## Where to read next

The documents are in Japanese for now. English is the first language of the toolchain, and the compiler's output and the documents will move to it.

| You want to | Read |
|---|---|
| Know what the language is for | [`docs/requirements.md`](docs/requirements.md), "価値と位置づけ" |
| Write a specification | [`docs/grammar.md`](docs/grammar.md), [`docs/requirements.md`](docs/requirements.md) "書き方の指針", [`docs/vocab.md`](docs/vocab.md), [`docs/config.md`](docs/config.md) |
| Run the commands, or set up an editor | [`docs/cli.md`](docs/cli.md), [`docs/editors.md`](docs/editors.md) |
| Write a plugin | [`docs/plugin-protocol.md`](docs/plugin-protocol.md), [`docs/ir.md`](docs/ir.md), [`schema/ir.schema.json`](schema/ir.schema.json) |
| Work on Hozo itself | [`CLAUDE.md`](CLAUDE.md), then [`docs/README.md`](docs/README.md) for which document decides what |
| Know what is decided, and what is not | [`docs/decisions.md`](docs/decisions.md), [`docs/open.md`](docs/open.md) |
