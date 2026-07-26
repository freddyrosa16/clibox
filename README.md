# clibox

Terminal email.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/freddyrosa16/clibox/main/install.sh | sh
```

## Use

```sh
clibox
clibox doctor
```

`clibox` remembers the last account, mailbox, search, and email you were reading.

Config: `~/.config/clibox/config.toml`  
State: `~/.local/state/clibox/clibox.db`

## Dev

```sh
go test ./...
```
