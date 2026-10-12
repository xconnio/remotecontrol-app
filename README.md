# remotecontrol-app

Remote control for a [Deskconn](https://github.com/xconnio/deskconn) machine: screen brightness and lock,
screenshots, media players (MPRIS), audio mute and printing.

It runs next to `deskconnd` on the machine and registers its procedures on deskconnd's local socket
(`~/.deskconn/deskconn.sock`). deskconnd's xlink forwards remote clients' calls to them, so the web and mobile apps
reach them under the same procedure names as before. If deskconnd isn't up yet, or restarts, the app keeps
retrying the connection.

On a desktop (`DISPLAY` or `WAYLAND_DISPLAY` set) everything is served; on a headless server only printing is.

## Procedures

| Procedure                                            | Needs a desktop |
|------------------------------------------------------|-----------------|
| `io.xconn.deskconn.deskconnd.screen.brightness.get`  | yes             |
| `io.xconn.deskconn.deskconnd.screen.brightness.set`  | yes             |
| `io.xconn.deskconn.deskconnd.screen.lock`            | yes             |
| `io.xconn.deskconn.deskconnd.screen.islocked`        | yes             |
| `io.xconn.deskconn.deskconnd.screenshot`             | yes             |
| `io.xconn.deskconn.deskconnd.screenshot.permission`  | yes             |
| `io.xconn.deskconn.deskconnd.mpris.*`                | yes             |
| `io.xconn.deskconn.deskconnd.audio.*`                | yes             |
| `io.xconn.deskconn.deskconnd.printer.list`           | no              |
| `io.xconn.deskconn.deskconnd.printer.print`          | no              |

Screenshots and printing are off until enabled in `~/.deskconn/config.yml` (`screenshot.enabled`,
`printing.mode`).

## Build and run

```bash
make build      # bin/remotecontrol-app
make run
```
