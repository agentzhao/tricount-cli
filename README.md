# tricount

Command-line client for [Tricount](https://tricount.com) expense groups. It speaks the same unofficial registry API as [tricount-api](https://pypi.org/project/tricount-api/). The module lives at [github.com/agentzhao/tricount-cli](https://github.com/agentzhao/tricount-cli).

Output is JSON, with a `summary` and a `next` list on every successful command, so an agent can start at `tricount --help` and walk one level at a time. `--human` prints the summary as text. Errors go to stderr.

A group is identified by the sharing token in `https://tricount.com/tABC123xyz`. Anyone with that token can read and edit the group. The first API call creates device credentials at `~/.config/tricount/credentials.json` (override with `--credentials` or `TRICOUNT_CREDENTIALS`). Amounts are positive major units, such as `12.50` or `1500`, never cents.

## Commands

```
tricount
├── attachment
│   ├── add                  Link a receipt id to a transaction
│   ├── gallery
│   │   ├── delete           Delete a gallery image by UUID (--yes)
│   │   ├── list             List gallery images
│   │   └── upload           Upload a gallery image
│   ├── remove               Unlink a receipt (--yes)
│   └── upload               Upload a receipt and print its id
├── auth
│   ├── reset                Delete device credentials (--yes)
│   ├── status               Show the credentials path (offline)
│   └── whoami               Authenticate and print the device user
├── balance
│   └── show                 Balances and suggested payments
├── category
│   └── list                 Expense categories, group categories, custom labels
├── expense                  Alias: transaction, tx
│   ├── add                  Equal split
│   ├── delete               Delete a transaction (--yes)
│   ├── edit                 Edit any transaction
│   ├── get                  Read one transaction
│   ├── list                 List transactions
│   ├── ratio                Split by integer ratios
│   └── split                Exact amount per member
├── group
│   ├── archive              Make a group read-only
│   ├── create               Create a group (--title --currency)
│   ├── delete               Permanently delete a group this device created (--yes)
│   ├── get                  Members, transactions, and balances
│   ├── join                 Sync a share link onto this device
│   ├── leave                Remove a group from this device (--yes)
│   ├── list                 Groups already synced here
│   ├── sync                 Fetch several tokens at once
│   ├── unarchive            Make an archived group editable
│   └── update               Change title, emoji, or category
├── income
│   └── add                  Record income split equally
├── member
│   ├── add                  Add people (--name, repeatable)
│   ├── delete               Remove a member (--yes)
│   ├── link                 Which member this device represents in the app
│   ├── list                 Names, uuids, ids, status
│   └── rename               Change a display name
├── rate
│   ├── get                  One exchange rate (--from --to)
│   └── list                 Rates from one currency
├── reimbursement            Alias: reimburse
│   └── add                  Record a payment between two members
└── version                  Print version, commit, and platform
```

Global flags: `--credentials`, `--human`, `--yes`, `-h` / `--help`, `--version`.

## Setup

From this directory, with [just](https://github.com/casey/just):

```bash
just test
just build          # ./tricount
just install        # GOPATH/bin
just dev group --help
```

Without just:

```bash
go test ./...
go build -o tricount ./cmd/tricount
```

## Release

Tag the commit, push it, then publish from this directory. [GoReleaser](https://goreleaser.com/install/) builds `tricount` for Linux, macOS, and Windows and creates the GitHub release. Piped stdin becomes the release notes.

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin main
git push origin v0.1.0
just publish <<'EOF'
## Changes
- ...
EOF
```

## Workflow

```bash
tricount group create --title "Flat" --currency EUR
tricount member add --token tABC123xyz --name Alice --name Bob
tricount expense add --token tABC123xyz --description Dinner --amount 42.50 --payer Alice --among Alice,Bob
tricount balance show --token tABC123xyz
```

`tricount group join --token tABC123xyz` opens an existing share link. Destructive commands (`group delete`, `group leave`, `member delete`, `expense delete`, `attachment remove`, `attachment gallery delete`, `auth reset`) require `--yes` and never prompt.
