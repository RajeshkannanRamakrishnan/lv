# Log Viewer (lv)

`lv` is a fast and interactive command-line log viewer built with Go and Bubbletea.

## Features

- **Interactive TUI**: Scroll through logs with ease (vim-like bindings: `j`, `k`, `g`, `G`, etc).
- **Time Travel**: Jump to specific times using `J` (supports formats like "14:30" or full timestamps).
- **Advanced Filtering**:
    - Text search with `/`.
    - Regex support (toggle with `Ctrl+r`).
    - Date range filtering (`[` and `]`).
    - Log Level toggles (Error, Warn, Info, Debug).
- **Live Tailing**: Follow mode (`f`) to automatically scroll to new logs.
- **Stack Trace Folding**: Toggle (`z`) to fold/unfold indented stack trace blocks.
- **Timeline View**: Visual timeline (`t`) overlay to see log distribution.
- **Bookmarks**: Mark interesting lines (`m`) and navigate between them (`n`/`N`).
- **Responsive**: Adapts to terminal window size with Toggleable Word Wrap (`w`).
- **Mouse Support**: continuous scrolling, selection, and copying.
- **Pipe Support**: Can read from `stdin` (e.g., `cat file.log | lv`).

## Installation

```bash
go install github.com/rajeshkannanramakrishnan/lv@latest
```

Or build from source:

```bash
git clone https://github.com/rajeshkannanramakrishnan/lv.git
cd lv
go build -o lv main.go
```

## Usage

```bash
# Open a file
./lv app.log

# Pipe content
cat app.log | ./lv
```

### Keybindings

| Key | Action |
| --- | --- |
| `j` / `Down` | Scroll down |
| `k` / `Up` | Scroll up |
| `h` / `Left` | Scroll Left |
| `l` / `Right` | Scroll Right |
| `g` / `Home` | Go to Top |
| `G` / `End` | Go to Bottom |
| `d` / `Ctrl+d` | Scroll down half page |
| `u` / `Ctrl+u` | Scroll up half page |
| `/` | Enter filter mode |
| `Ctrl+r` | Toggle Regex Mode for filter |
| `Enter` | Apply filter |
| `Esc` | Clear filter / Cancel / Clear Selection |
| `[` | Set Start Date Filter |
| `]` | Set End Date Filter |
| `J` | Jump to Time (Time Travel) |
| `1` | Toggle ERROR logs |
| `2` | Toggle WARN logs |
| `3` | Toggle INFO logs |
| `4` | Toggle DEBUG logs |
| `f` | Toggle Follow Mode (Live Tailing) |
| `w` | Toggle Word Wrap |
| `z` | Toggle Stack Trace Folding |
| `t` | Toggle Timeline View |
| `m` | Toggle Bookmark on current line |
| `n` | Go to Next Bookmark |
| `N` | Go to Previous Bookmark |
| `y` | Copy current selection to clipboard |
| `q` / `Ctrl+c` | Quit |

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
