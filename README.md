# Hozo

English | [日本語](README.ja.md)

## The name

**Hozo** (枘) is the Japanese word for a tenon: the tongue cut on the end of a piece of wood, which fits into a hole in another piece. A joiner who builds with tenons does not match the insides of the wood. Only the shapes where two pieces meet have to match, and the pieces hold together.

Hozo does the same to the design of software. You do not write the insides. You decide only the shapes where the parts meet: what each part takes and returns, what it reads and writes, and what it calls in which order.

Those shapes are the design, and Hozo is the language that holds them as **the single source of truth**. When an AI writes most of the code, the design is what people still have to own. Hozo keeps it in a form that people can review and correct.

## Design methods, written as code

What you draw with a design method is one view of those shapes. In Hozo, the views are written in one language, in one specification.

| Design method | What you write in Hozo |
|---|---|
| Requirements list, traceability matrix | A requirement is a case of a classification. `@prd.ref` ties the design to it |
| Use case diagram, use case description | An actor is a case of a classification. A use case is a function with `@prd.usecase.of`. Its flow is the wiring of the function |
| Business flow diagram | The wiring of a function with `@prd.business.flow`: the use cases, in order |
| Activity diagram, sequence diagram, decision table | Wiring: the order of the calls, `match` over a classification, `disjoint`, `for` |
| State machine diagram, state transition table | The cases of a classification, and the transitions that an effect declares |
| Class diagram, ER diagram | `model`, `state`, `@valid.*`, `@db.*` |
| Component diagram, API design | Namespaces, `export`, `@http.*` |

The full correspondence, with the parts that Hozo cannot write yet, is in [`docs/ide-chain.md`](docs/ide-chain.md), "対応".

Three pieces of the sample [`tests/samples/shop-server`](tests/samples/shop-server), abridged, with the comments translated:

```
// Use case diagram and business flow: who does what, in which order.
classify Actor of never {
  @prd.actor.human Customer;
}

@prd.usecase.of(design.Actor.Customer)
export fn order(token: type.Token, lines: type.OrderLine[]): type.Order {
  return server.placeOrder({ token: token, lines: lines });
}

@prd.business.flow
export fn signInAndOrder(email: type.Email, password: type.Password, lines: type.OrderLine[]): Ordered {
  val token = client.signIn(email, password);
  val placed = client.order(token, lines);
  val listed = client.viewOrders(token);
  return { placed: placed, listed: listed };
}
```

```
// Activity diagram and decision table: the cases, and what happens in each.
classify Credential of type.Token? {
  Presented: type.Token? as type.Token;
  Absent:    type.Token? as else;
}

@prd.ref(design.Requirement.PRD_001)
fn authenticate(token: type.Token?): type.Session {
  return match (token by Credential) {
    Presented => sessionOf(token);
    Absent    => throw type.Unauthorized;
  }
}
```

```
// Class diagram and ER diagram: the things, their constraints, and where they are kept.
model User {
  @db.key    @db.text id: UserId;
  @db.unique @db.text email: Email;
  @db.text @valid.length(min: 1, max: 100) name: string;
}

@db.table
state users: list<type.User>;
```

## What the design becomes

`hozoc` checks the specification, and turns it into the skeleton of the code and its test harness.

- The wiring is generated as code.
- A function without a body is left to a person or an AI. If its implementation is missing, the generated code does not compile.
- The tests that the specification requires are determined from it, and `hozoc` checks that they ran.

The first target language is Go. A pair of a specification and its code is in [`tests/samples/shop-server`](tests/samples/shop-server).

## Status

Hozo is unreleased and at version 0.x. Nothing is compatible from one version to the next yet.

- **Exists**: the compiler `hozoc`, the language server, the VS Code extension, and the plugins for Go.
- **Does not exist yet**: the UI for drawing, reviewing and correcting a design. Target languages other than Go. A license.

## Read next

| You want to | Read |
|---|---|
| Know what the language is for | [`docs/requirements.md`](docs/requirements.md), "価値と位置づけ" |
| Write a specification | [`docs/grammar.md`](docs/grammar.md), and [`docs/requirements.md`](docs/requirements.md), "書き方の指針" |
| Run `hozoc` | [`docs/cli.md`](docs/cli.md). `make build` builds it into `bin/`, with Go 1.24.7 or later |
| Write a plugin | [`docs/plugin-protocol.md`](docs/plugin-protocol.md) |
| Work on Hozo itself | [`CLAUDE.md`](CLAUDE.md), then [`docs/README.md`](docs/README.md) |

The documents are in Japanese for now. They will move to English, the first language of the toolchain.
