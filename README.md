# x

Generic Go libraries shared across Discobox projects. Nothing here knows about
Discobox itself; each package stands alone and is useful outside it.

| Package | What it is |
| --- | --- |
| [`config`](config) | A server's configuration from one file plus environment overrides: the Config struct is the source of truth, env names derive from key paths (`<PREFIX>_<KEY_PATH>`), unknown keys and misspelled variables fail, secrets accept `file:<path>`, and it generates the JSON schema and commented example file. |
| [`gormdb`](gormdb) | GORM connection pools for SQLite, Postgres, and Turso, opened from a DSN. SQLite gets the split write/read pool that WAL wants: one writer with `_txlock=immediate`, many readers with `mode=ro`. |
| [`frontmatter`](frontmatter) | A script with a YAML metadata block at the top, delimited by `---`, `#---` or `//---`, and a stable id derived from its filename. Normalizes key spelling and value shape; what the fields mean is the reader's. |
| [`gitutil`](gitutil) | Running `git` as a subprocess and reading what it says — repository roots, status, refs. |
| [`id`](id) | Prefixed random identifiers: `<prefix>_<16 Crockford base32 chars>`, plus short-form resolution and the hyphenated spelling an ID takes in a hostname. |
| [`selection`](selection) | Mouse gestures over a cell grid turned into a text selection: drag, double-click for a word, triple-click for a line, block mode. Reports the spans to highlight and the text they hold; it draws nothing and touches no clipboard. |
| [`shorttmp`](shorttmp) | A temporary directory short enough to hold a Unix socket, for tests that bind one. |

## Versioning

Consumed at the latest commit on `main`; there are no tags yet.

```
go get github.com/discobox-ai/x@main
```
