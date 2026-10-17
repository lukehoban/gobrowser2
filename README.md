# gobrowser2

A small web browser written from scratch in Go. It covers networking, HTML and CSS parsing, style, layout, and painting, and outputs a PNG of the rendered page. The target is to render as much of [Hacker News](https://news.ycombinator.com/) as possible. Plan and progress are tracked in [#1](https://github.com/lukehoban/gobrowser2/issues/1).

> **No render yet.** A screenshot of the current HN render will go here once painting lands ([#7](https://github.com/lukehoban/gobrowser2/issues/7)).

## What works

- **HTML parsing** ([`html/`](html)): a hand-written tokenizer and a simplified tree builder. It handles implied `html`/`head`/`body`, void elements, optional end tags, implied `tbody`/`tr`, table-scoped recovery, foster parenting, quirks-mode detection, and character references.
- **CLI**: `gobrowser -dump=dom <file>` prints the parsed DOM tree.

## What's next

| Stage | Issue |
|---|---|
| Networking (HTTP/1.1 over TCP/TLS) | [#2](https://github.com/lukehoban/gobrowser2/issues/2) |
| CSS parser | [#5](https://github.com/lukehoban/gobrowser2/issues/5) |
| Style computation | [#3](https://github.com/lukehoban/gobrowser2/issues/3) |
| Block layout | [#11](https://github.com/lukehoban/gobrowser2/issues/11) |
| Inline layout and text | [#4](https://github.com/lukehoban/gobrowser2/issues/4) |
| Table layout | [#6](https://github.com/lukehoban/gobrowser2/issues/6) |
| Paint to PNG | [#7](https://github.com/lukehoban/gobrowser2/issues/7) |
| Images | [#9](https://github.com/lukehoban/gobrowser2/issues/9) |
| HN golden render in CI | [#12](https://github.com/lukehoban/gobrowser2/issues/12) |

## Usage

```sh
go run ./cmd/gobrowser -dump=dom html/testdata/hn_like.html
```

```
#document (quirks)
  <html lang="en" op="news">
    <head>
      <meta name="referrer" content="origin">
      <link rel="stylesheet" type="text/css" href="news.css">
      <title>
        "Example & News"
    <body>
      <center>
        <table id="hnmain" border="0" cellpadding="0" cellspacing="0" width="85%" bgcolor="#f6f6ef">
          <tbody>
            <tr>
              <td bgcolor="#ff6600">
              ...
```

## Development

```sh
gofmt -l .        # formatting (CI fails if any files are listed)
go vet ./...
go test ./...
```
