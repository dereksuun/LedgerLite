# LedgerLite

bank mini-core with events + idempotence

## GUI

The GUI is an HTTP client for the existing API. Start the backend first:

```bash
make dev
```

Build GUI for current OS:

```bash
make gui
```

On Linux/macOS, run `bin/ledgerlite-gui` and it serves a local UI (default `http://127.0.0.1:8787`) with API proxying.

Build Windows executable (`.exe`):

```bash
make gui-win
```

Output:

- `bin/ledgerlite-gui` (Linux/macOS local GUI server)
- `bin/ledgerlite-gui.exe` (Windows amd64)

The GUI lets you:

- check `/health`
- create accounts
- list accounts
- create transfers (with idempotency key)
- load account statement
