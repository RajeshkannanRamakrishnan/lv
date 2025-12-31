# Log Viewer (lv)

`lv` is a fast and interactive command-line log viewer built with Go and Bubbletea.

## Features

- **Interactive TUI**: Scroll through logs with ease (vim-like bindings: `j`, `k`, `g`, `G`, etc).
- **Filtering**: Press `/` to search/filter lines containing a string.
- **Responsive**: Adapts to terminal window size.
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
| `d` / `Ctrl+d` | Scroll down half page |
| `u` / `Ctrl+u` | Scroll up half page |
| `/` | Enter filter mode |
| `Enter` | Apply filter |
| `Esc` | Clear filter / Cancel |
| `q` / `Ctrl+c` | Quit |

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
