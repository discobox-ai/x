# x

Generic Go libraries shared across Discobox projects. Nothing here knows about
Discobox itself; each package stands alone and is useful outside it.

| Package | What it is |
| --- | --- |
| [`gormdb`](gormdb) | GORM connection pools for SQLite, Postgres, and Turso, opened from a DSN. SQLite gets the split write/read pool that WAL wants: one writer with `_txlock=immediate`, many readers with `mode=ro`. |
| [`frontmatter`](frontmatter) | A script with a YAML metadata block at the top, delimited by `---`, `#---` or `//---`, and a stable id derived from its filename. Normalizes key spelling and value shape; what the fields mean is the reader's. |
| [`gitutil`](gitutil) | Running `git` as a subprocess and reading what it says — repository roots, status, refs. |
| [`id`](id) | Prefixed random identifiers: `<prefix>_<16 Crockford base32 chars>`, plus short-form resolution. |
| [`selection`](selection) | Mouse gestures over a cell grid turned into a text selection: drag, double-click for a word, triple-click for a line, block mode. Reports the spans to highlight and the text they hold; it draws nothing and touches no clipboard. |
| [`shorttmp`](shorttmp) | A temporary directory short enough to hold a Unix socket, for tests that bind one. |

## Versioning

Consumed at the latest commit on `main`; there are no tags yet.

```
go get github.com/discobox-ai/x@main
```
