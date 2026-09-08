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
 curl -s -k -u 'admin:********' -H 'Accept: application/json' 'https://10.0.0.5/redfish/v1/Systems…
 root > Systems > 1
┌ Links (14) ──────────────┬ Response ──────────────────────────────────────────────────────────┐
│  ..                      │ HTTP/1.1 200 OK                                                    │
│ ── Resource ──           │ Content-Type: application/json;charset=utf-8                       │
│  Bios                    │ OData-Version: 4.0                                                 │
│  Storage                 │                                                                    │
│ ── Links ──              │ {                                                                  │
│  Chassis[0]              │   "@odata.id": "/redfish/v1/Systems/1",                            │
│ ── Actions ──            │   "Id": "1",                                                       │
│  #ComputerSystem.Reset ⚡ │   "Name": "Contoso Server",                                        │
│ ── Oem · Hpe ──   (oem)  │   "PowerState": "On",                                              │
│▸ Thermal           (oem) │   "Bios": {                                                        │
│  SmartStorage      (oem) │     "@odata.id": "/redfish/v1/Systems/1/Bios"                      │
└──────────────────────────┴────────────────────────────────────────────────────────────────────┘
 /redfish/v1/Systems/1/Oem/Hpe/Thermal        tab panes · L location · r reload · ? help · q quit
```

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

An optional trailing argument is the resource to start at, so a path from a bug
report can be opened directly:

```sh
rfx -H 10.0.0.5 -u admin -k /redfish/v1/Systems/1
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

rfx fails before it takes over the terminal: an unreachable host, an unverified
certificate or wrong credentials are reported on a plain terminal and exit 1.

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
| `backspace` / `left`      | go back to the previous location                   |
| `L`                       | edit the current endpoint, `enter` to load it      |
| `r`                       | reload the current location, bypassing the cache   |
| `y`                       | copy the `curl` command to the clipboard           |
| `page up` / `page down`   | scroll the response pane                           |
| `?`                       | help overlay                                       |
| `q` / `ctrl+c`            | quit                                               |

`backspace` and `left` retrace the trail the user actually walked, which is not
the same as the path tree: a link can lead out of the current subtree, and going
back returns to where it was followed. To move up the path instead, follow the
`..` entry at the top of the link pane.

## What it shows

The left pane lists everything the current response links to, grouped by where
in the document it was found: `Resource`, `Members`, `Links`, `Actions`,
`Annotations`, one group per OEM vendor, and `Headers`. Within a group the links
keep the order the service wrote them in.

Beyond the obvious `@odata.id` values, rfx surfaces the things that are easy to
miss by hand:

- **Action targets**, `Actions.Oem` ones included, marked `⚡`. They answer to
  POST rather than GET, so they are listed to be discovered; following one opens
  its `@Redfish.ActionInfo` when it advertises one.
- **Path-like strings** under plain `Uri`, `href` or `Target` keys, which some
  vendors use instead of a proper navigation link.
- **Redfish annotations** — `@Redfish.Settings`,
  `@Redfish.CollectionCapabilities` and friends — a common hiding place for
  vendor behaviour.
- **`Location` and `Content-Location` response headers**, when they point at a
  Redfish resource.

**OEM material is the point.** Anything below an `Oem` key is drawn in its own
colour with an `(oem)` suffix, grouped under an `Oem · <Vendor>` heading, and
highlighted in the raw JSON too, so a vendor block is as obvious in the body as
in the link list. The vendor name comes from the document, never from a built-in
list: the extensions worth finding are the ones nobody has a list of.

The header carries the `curl` command for the current location on a single line,
between the path and the breadcrumb, so it can be selected and copied in one
gesture. `y` copies it outright, which also gets around the line being truncated
on a narrow terminal. It omits the `User-Agent` header rfx sets on the real
request: `curl` sends its own, and no service answers differently because of it.

`y` copies by two routes at once, because neither covers every case. OSC 52
travels down an SSH connection, which is how a BMC is usually reached, but many
terminals refuse to act on it: VTE-based ones (GNOME Terminal, Tilix,
Terminator) never have, and `tmux` (`set -g set-clipboard on`) and `xterm`
(`allowWindowOps`) need it turned on. The local clipboard always works, but only
on the machine rfx itself runs on, and on Linux it needs `xclip`, `xsel` or
`wl-copy` installed.

The footer says which of the two got through: `copied to the clipboard` means
the local one was written and the text is definitely there, while `sent as
OSC 52` means only the blind route was left and the terminal may have dropped
it.

The right pane shows the response status and headers, and the pretty-printed
body. Keys stay in the order the service sent them, because Redfish services
order them meaningfully.

`L` opens the location bar for typing or pasting a path. A full URL copied from
a browser is accepted and reduced to its path; one naming a different host is
refused. A path that turns out not to exist simply renders its 404 — probing for
undocumented endpoints is a first-class use.

Responses are cached for `--cache-ttl` (5 minutes by default), so walking back up
the tree is instant. The header says `cached 12s ago` whenever a view is not
live, and `r` forces a fresh request.

## Development

```sh
task install           # build the pinned toolchain into bin/
task install-githooks  # install the lefthook hooks
task build
task test
task lint              # needs: npm install -g markdownlint-cli2
task format
```
