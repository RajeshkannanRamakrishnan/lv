# Log Viewer (lv)

`lv` is a fast, interactive, and modern command-line log viewer built with Go and Bubbletea. It's designed to make analyzing large log files effortless with vim-like navigation, powerful filtering, and time-travel capabilities.

## Features

*   **⚡ Fast & Interactive**: Smooth scrolling and navigation, even for large files.
*   **🔍 Powerful Filtering**:
    *   **Text Search**: Standard search (`/`) with regex support (`Ctrl+r`).
    *   **Date Range**: Filter logs between specific dates (`[` and `]`).
    *   **Log Levels**: Quickly toggle visibility of ERROR, WARN, INFO, and DEBUG logs.
*   **⏰ Time Travel**: Jump instantly to a specific time (e.g., "14:30") using `J`.
*   **👀 Live Monitoring**:
    *   **Follow Mode**: Auto-scroll to new logs (`f`), similar to `tail -f`.
    *   **Timeline View**: Visualize log distribution over time (`t`).
*   **🧠 Smart Analysis**:
    *   **Stack Trace Folding**: Collapse complex stack traces (`z`) for better readability.
    *   **Bookmarks**: Mark important lines (`m`) and navigate between them (`n`/`N`).
*   **💻 Developer Friendly**:
    *   **Vim-bindings**: Natural navigation for vim users (`j`, `k`, `g`, `G`).
    *   **Pipe Support**: Pipe logs directly: `cat app.log | lv`.
    *   **Responsive**: Adapts to any terminal size with toggleable word wrap (`w`).

## Installation

### Option 1: Quick Install (Recommended)

You can install `lv` using the provided installation script:

```bash
./install.sh
```

This will build the binary and move it to `/usr/local/bin` (may require sudo).

### Option 2: Go Install

If you have Go installed:

```bash
go install github.com/rajeshkannanramakrishnan/lv@latest
```

### Option 3: Build from Source

```bash
git clone https://github.com/rajeshkannanramakrishnan/lv.git
cd lv
go build -o lv main.go
sudo mv lv /usr/local/bin/
```

## Usage

**Open a file:**
```bash
lv app.log
```

**Read from stdin:**
```bash
cat app.log | lv
kubectl logs pod-name | lv
```

## Keybindings

### 🧭 Navigation
| Key | Action |
| :--- | :--- |
| `j` / `Down` | Scroll down |
| `k` / `Up` | Scroll up |
| `d` / `Ctrl+d` | Scroll down (half page) |
| `u` / `Ctrl+u` | Scroll up (half page) |
| `g` / `Home` | Go to Top |
| `G` / `End` | Go to Bottom |
| `m` | Toggle Bookmark |
| `n` / `N` | Next / Previous Bookmark |

### 🔍 Search & Filter
| Key | Action |
| :--- | :--- |
| `/` | Start Search |
| `Ctrl+r` | Toggle Regex Search |
| `Esc` | Clear Filter / Cancel |
| `[` / `]` | Set Start / End Date Filter |
| `1` - `4` | Toggle ERROR / WARN / INFO / DEBUG |

### 🛠 Tools & Display
| Key | Action |
| :--- | :--- |
| `J` | **Time Travel** (Jump to time) |
| `f` | Toggle **Follow Mode** (Live tail) |
| `t` | Toggle **Timeline View** |
| `z` | Toggle **Stack Trace Folding** |
| `w` | Toggle Word Wrap |
| `y` | Copy selection to clipboard |
| `q` | Quit |

## License

MIT License - see the [LICENSE](LICENSE) file for details.
