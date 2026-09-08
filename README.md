# rfx — Redfish Explorer

`rfx` is a terminal UI for exploring the Redfish API of a BMC.

What a server actually implements — Redfish version, how much of the standard
tree is populated, and above all which OEM extensions exist — differs by vendor,
by model and by firmware version. OEM extensions are the hardest part: they are
barely documented and they hide in `Oem` sub-objects scattered anywhere in the
resource tree.

`rfx` replaces the `curl | jq` loop. It drills down the live API by following
every link it can find in a response, highlights OEM material, and always shows
the exact `curl` command for the current location, so a finding can be pasted
straight into a bug report or a script.

```text
 /redfish/v1/Systems/1                                      RedfishVersion 1.18.0 · 200 OK · 41ms
 root > Systems > 1
┌ Links (14) ──────────────┬ Response ──────────────────────────────────────────────────────────┐
│  ..                      │ curl -s -k \                                                       │
│ ── Resource ──           │   -u 'admin:********' \                                            │
│  Bios                    │   -H 'Accept: application/json' \                                  │
│  Storage                 │   'https://10.0.0.5/redfish/v1/Systems/1'                          │
│ ── Links ──              │                                                                    │
│  Chassis[0]              │ HTTP/1.1 200 OK                                                    │
│ ── Actions ──            │ Content-Type: application/json;charset=utf-8                       │
│  #ComputerSystem.Reset ⚡│ OData-Version: 4.0                                                 │
│ ── Oem · Hpe ──   (oem)  │                                                                    │
│▸ Thermal           (oem) │ {                                                                  │
│  SmartStorage      (oem) │   "@odata.id": "/redfish/v1/Systems/1",                            │
└──────────────────────────┴────────────────────────────────────────────────────────────────────┘
 /redfish/v1/Systems/1/Oem/Hpe/Thermal        tab panes · L location · r reload · ? help · q quit
```

## Status

Under construction.

## Install

```sh
go install github.com/breml/redfish-explorer/cmd/rfx@latest
```

Or from a checkout:

```sh
task install   # build the pinned toolchain into bin/
task build     # binary at bin/rfx
```

## Usage

```sh
rfx --host 10.0.0.5 --username admin --password secret --insecure
```

| Flag              | Short | Default         | Description                                                     |
|-------------------|-------|-----------------|-----------------------------------------------------------------|
| `--host`          | `-H`  | —               | Redfish host: `10.0.0.5`, `10.0.0.5:8443` or `https://10.0.0.5` |
| `--username`      | `-u`  | —               | user name to authenticate with                                  |
| `--password`      | `-p`  | `$RFX_PASSWORD` | password                                                        |
| `--insecure`      | `-k`  | `false`         | skip TLS certificate verification                               |
| `--cache-ttl`     |       | `5m`            | how long to cache visited endpoints; `0` disables the cache     |
| `--show-password` |       | `false`         | show the real password in the rendered curl command             |
| `--version`       |       |                 | print the version and exit                                      |

BMCs almost always present a self-signed certificate, so `--insecure` is usually
required. It is an explicit opt-in rather than a default.

A password passed as `--password` is visible in `ps` and in the shell history.
Set `RFX_PASSWORD` instead to avoid that.

The rendered `curl` command masks the password as `********`. Pass
`--show-password` to render it verbatim — useful when the command is meant to be
run, unwise when the screen is being shared.

## Keys

| Key                       | Action                                             |
|---------------------------|----------------------------------------------------|
| `tab`                     | switch between the link pane and the response pane |
| `up` / `down` / `j` / `k` | move through the links                             |
| `enter`                   | follow the selected link                           |
| `backspace`               | go one level up                                    |
| `L`                       | edit the current endpoint, `enter` to load it      |
| `r`                       | reload the current location, bypassing the cache   |
| `page up` / `page down`   | scroll the response pane                           |
| `?`                       | help                                               |
| `q` / `ctrl+c`            | quit                                               |

OEM links are shown in their own colour with an `(oem)` suffix, grouped under
`Oem · <Vendor>` headers. Action targets are marked `⚡`; they are POST-only and
are listed for discovery.

## Development

```sh
task install           # build the pinned toolchain into bin/
task install-githooks  # install the lefthook hooks
task build
task test
task lint              # needs: npm install -g markdownlint-cli2
task format
```

## Planned

Search within the response body; jq-style filtering; `POST`/`PATCH`/`DELETE`
with an editor dialog; a config file storing hosts and credentials per endpoint;
a script mode that submits a change and polls the returned task monitor; session
authentication (`X-Auth-Token`); reading the password from a file, from stdin,
or from an interactive prompt.
