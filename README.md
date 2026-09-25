# tricount

Command-line client for [Tricount](https://tricount.com) expense groups. It speaks the same unofficial registry API as [tricount-api](https://pypi.org/project/tricount-api/). The module lives at [github.com/agentzhao/tricount-cli](https://github.com/agentzhao/tricount-cli).

Output is JSON, with `ok` and `data` on every successful command. `--help` on each command lists the usual next commands. `--human` prints a one-line summary as text. Errors go to stderr. `--json-errors` writes them as JSON on stderr:

```json
{"ok":false,"error":{"code":"member_not_found","message":"no member \"Cara\"","hint":"List them with: tricount member list"}}
```

Stable codes include `usage`, `missing_target`, `invalid_target`, `missing_amount`, `invalid_amount`, `invalid_exchange_rate`, `invalid_filter`, `member_not_found`, `ambiguous_member`, `missing_member`, `transaction_not_found`, `confirmation_required`, `group_archived`, `idempotency_conflict`, `invalid_idempotency_key`, `unknown_profile`, `config_error`, `api_error`, and `error`.

A group is identified by the sharing token in `https://tricount.com/tABC123xyz`. Anyone with that token can read and edit the group. The first API call creates device credentials at `~/.config/tricount/credentials.json` (override with `--credentials` or `TRICOUNT_CREDENTIALS`). Amounts are positive exact decimals in major units, such as `12.50` or `1500`, never cents. More than two decimal places (`1.005`) is rejected. Exchange rates are exact decimals too.

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
│   ├── list                 List transactions (--since --until --type --member --limit --format)
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
│   ├── profiles             Named groups from the local config
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
├── update                   Replace this binary with a GitHub release (--yes)
└── version                  Print version, commit, and platform
```

Global flags: `--credentials`, `--config`, `--human`, `--json-errors`, `--yes`, `-h` / `--help`, `--version`.

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

`tricount group join --token tABC123xyz` opens an existing share link. Destructive commands (`group delete`, `group leave`, `member delete`, `expense delete`, `attachment remove`, `attachment gallery delete`, `auth reset`, `update`) require `--yes` and never prompt.

`expense add`, `expense split`, `expense ratio`, `income add`, and `reimbursement add` accept `--idempotency-key`. The key is scoped to the group and stored as a deterministic transaction UUID. A later run with the same key prints the existing transaction when the description, amount, payer, and allocations still match, and fails when they do not.

```bash
tricount income add --token tABC123xyz --description "Fun money" --amount 20 --receiver Alice --among Alice,Bob --idempotency-key fun-money:2026-10
```

Named groups live in `~/.config/tricount/config.toml` (`--config` or `TRICOUNT_CONFIG`). `token_env` names an environment variable so the share token is not written into shell scripts. `payer`, `receiver`, and `among` fill those flags when you pass `--group` and omit them. `--token` still works on its own.

```toml
[groups.fun]
token_env = "TRICOUNT_FUN_TOKEN"
payer = "Alice"
among = ["Alice", "Bob"]
```

```bash
tricount group profiles
tricount expense add --group fun --description Dinner --amount 12.50
```

`tricount update` installs the latest [GitHub release](https://github.com/agentzhao/tricount-cli/releases) over the binary you are running. `tricount update v0.1.0` installs that tag. A newer release also prints a notice on stderr at most once a day. Set `TRICOUNT_NO_UPDATE_NOTIFIER` to silence it.
