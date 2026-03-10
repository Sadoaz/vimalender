# vimalender

A vim-style terminal calendar built with Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

Navigate, create, move, search, and customize events entirely from the keyboard with familiar vim motions.


Demo:



https://github.com/user-attachments/assets/5c1090b0-922f-4f04-b4ba-83717fbde6fb






## Installation

### go install

Requires Go 1.25+. Installs the binary to your `$GOPATH/bin` (or `$GOBIN`):

```sh
go install github.com/Sadoaz/vimalender@latest
```

Make sure `$GOPATH/bin` is in your `PATH`, then run:

```sh
# bash/zsh
export PATH="$HOME/go/bin:$PATH"

# fish
fish_add_path ~/go/bin
```

```sh
vimalender
```

### From source

```sh
git clone https://github.com/Sadoaz/vimalender.git
cd vimalender
go build -o vimalender .
go install .
./vimalender
```

If you usually run the installed binary from `~/go/bin`, remember to run `go install .` after local changes.

## Highlights

- Week, month, and year views
- Vim-style navigation plus help overlay with live key rebinding
- Multi-day events, including recurring multi-day events and 24h+ all-day spans at the top of week view
- Visual selection, copy, cut, paste, delete, and grouped move
- Recurring move/delete flows with `o` = one occurrence and `a` = all occurrences
- Search with persistent match navigation
- In-app `.ics` import and export flows available from `Settings`
- In-app color customization for event color, consecutive-event color, event background, and UI accent

## Views

Vimalender has three main views:

| View | Key | Description |
|------|-----|-------------|
| **Week** | default | Multi-day column grid with time gutter. Shows 1-9 day columns. |
| **Month** | `M` | Full month grid with event dots and first event title per cell. ISO week numbers. |
| **Year** | `Y` | 4x3 grid of 12 mini-month calendars with event-day highlighting. |

Press `Enter` in month or year view to open the week view at the selected day.

## Keybindings

Press `?` any time in the main UI to open the help panel. It shows the current bindings, supports `Enter` to rebind a selected action, and `Backspace` to reset that binding to default.

### Navigation (Week View)

| Key | Action |
|-----|--------|
| `h` / `l` | Previous / next day |
| `j` / `k` | Move cursor down / up |
| `J` / `K` | Move cursor down / up by one visible row |
| `H` / `L` | Previous / next overlapping event |
| `Ctrl+D` / `Ctrl+U` | Jump quarter page down / up |
| `Tab` | Cycle through overlapping events at cursor |
| `1`-`9` | Set number of visible day columns |
| `c` | Jump to now (today on far left, cursor at current time) |
| `g` | Go to time -- type e.g. `14:30`, `1430`, `9` and press Enter |
| `G` | Go to day -- type day of month (1-31) and press Enter |
| `M` | Switch to month view |
| `Y` | Switch to year view |
| `S` | Open settings menu |
| `u` | Undo last action |
| `Ctrl+R` | Redo |
| `q` | Quit |

### Events

| Key | Action |
|-----|--------|
| `a` | Create new event at cursor position |
| `Enter` | Open detail view for event under cursor |
| `m` | Enter move mode |
| `e` | Edit event in `$EDITOR` (falls back to `vi`) |
| `dd` | Delete event (vim-style double-key) |
| `x` | Cut selected event |
| `yy` | Copy selected event |
| `p` | Paste clipboard at cursor |
| `u` | Undo last action (create, delete, move, edit) |
| `Ctrl+R` | Redo |

For recurring events:

- delete prompts: **(o)ne** occurrence or **(a)ll**
- move prompts: preview first, then `Enter`, then **(o)ne** or **(a)ll**

### Importing `.ics`

Open `Settings` with `S`, then choose `Import events`.

- type or paste a relative or absolute `.ics` path
- press `Tab` to complete matching folders and `.ics` files
- press `Enter` to import supported VEVENT entries into your local calendar
- unsupported recurrence rules, exceptions, and malformed events are skipped with reasons shown in the overlay
- duplicate prevention is not automatic, so importing the same file again creates new events

### Exporting `.ics`

Open `Settings` with `S`, then choose `Export events`.

- type or paste a relative or absolute `.ics` path
- press `Tab` to complete matching folders and `.ics` files
- press `Enter` to export your stored events to that file

### Create Flow

1. Press `a` to start creating at cursor
2. `j`/`k` to set duration by visible row, `J`/`K` to fine-tune by 1 minute, `Ctrl+D`/`Ctrl+U` to resize faster, `Enter` to confirm
3. Type title, `Enter`
4. Type description, `Enter` (skippable via settings)
5. `r`/`R` to cycle recurrence, `Enter` to save (skippable via settings)

Create spans can cross midnight and multiple days.

If a logical event is at least 24 hours long, it renders as a top all-day span in week view instead of as timed blocks in the grid.

### Search

| Key | Action |
|-----|--------|
| `/` | Start search (searches title and description) |
| `n` / `N` | Jump to next / previous match |
| `Ctrl+N` / `Ctrl+P` | Alternative next / previous match |
| `Esc` | Clear search highlights |

Search matches against event title and description. Navigate matches with `n`/`N`. When events are changed or deleted, the search list is refreshed so stale matches are removed.

### Visual Mode

Press `V` in week view to start a visual time-range selection.

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` | Expand the selection |
| `Ctrl+D` / `Ctrl+U` | Expand faster |
| `J` / `K` | Expand by exactly 1 minute |
| `y` | Copy selected events |
| `x` | Cut selected events |
| `d` | Delete selected events |
| `m` | Move selected events together |
| `Esc` / `V` | Leave visual mode |

Recurring selections are occurrence-aware:

- selecting one recurring occurrence does not automatically select the entire series
- visual recurring move previews first, then `Enter`, then `o` or `a`
- `o` means only the selected occurrences
- `a` means the whole recurring series

### Move Mode

Enter with `m` on any event. The event stays pinned to its visual column during move preview.

| Key | Action |
|-----|--------|
| `j` / `k` | Move event down / up by visible row |
| `J` / `K` | Move event down / up by 1 minute |
| `h` / `l` | Move event to previous / next day |
| `g` | Jump move preview to a typed time |
| `G` | Jump move preview to a typed day of month |
| `Enter` | Confirm move preview |
| `Esc` | Cancel move |

For recurring events, `Enter` opens the `(o)ne / (a)ll` prompt after you have already previewed the new position.

### Detail View

Press `Enter` on any event to view full details (title, description, date, time, duration, notes). Press `e` to edit in external editor, `Esc`/`q` to go back.

### Month & Year Views

| Key | Action |
|-----|--------|
| `h` / `l` | Move one day left / right |
| `j` / `k` | Move one week down / up |
| `H` / `L` | Move one month back / forward |
| `J` / `K` | Move one year back / forward (year view only) |
| `c` | Jump to today |
| `Enter` | Open week view at selected day |

## Recurrence

Events support seven recurrence patterns:

| Pattern | Description |
|---------|-------------|
| None | One-time event |
| Daily | Every day |
| Weekdays | Monday through Friday |
| Weekly | Same weekday each week |
| Biweekly | Same weekday every 2 weeks |
| Monthly | Same day of month |
| Yearly | Same month and day |

Recurring events can have an optional end date. Virtual occurrences are shown with a `↻` prefix.

Supported recurring workflows include:

- moving one occurrence or the whole series
- deleting one occurrence or the whole series
- recurring multi-day events
- recurring visual selections by occurrence

## Overlapping Events

Events can overlap in time. Overlapping events are displayed side-by-side in sub-columns within the same day. Use `H`/`L` or `Tab` to navigate between them. When scrolling with `j`/`k`, the cursor automatically selects new events as they appear. The status bar shows `[N/M: title]` to indicate which overlapping event is selected.

## Settings

Open with `S`. Navigate with `j`/`k`, change values with `h`/`l` or `Enter`.

| Setting | Default | Description |
|---------|---------|-------------|
| Default zoom | `30 min/row` | Default week-view zoom level |
| Event color | `#1a5fb4` | Main border/accent color for events |
| Consecutive event color | `#26a269` | Alternating border color for back-to-back event chains |
| Accent color | `#00a8ff` | Main UI accent (`WEEK`, headers, help, prompts, etc.) |
| Event background | `#1c1c2e` | Dark background fill for event blocks |
| Show keybinding hints | Off | Show shortcut hints in the status bar and menus |
| Show event borders | On | Left color bar style vs full background |
| Show descriptions | On | Show event descriptions below title in grid |
| Quick create | Off | Skip recurrence picker during create |
| Skip description | Off | Skip description step during create |

Color rows open an inline hex editor:

- type or paste a hex value like `#00a8ff`
- `Enter` saves
- `Ctrl+R` resets to the default
- `Esc` cancels

Title and description input also support `Ctrl+W` to delete the previous word.

## Customization

`settings.json` is autogenerated with helpful customization fields.

- `keybindings` contains the active binding for each action key
- `keybinding_help` explains what each binding does
- `ui_colors` controls accent-related UI colors
- `ui_color_help` explains each color key

You can either edit the JSON file directly or use the in-app help/settings flows.

## Data Storage

All data is stored locally in `~/.local/share/vimalender/`:

| File | Content |
|------|---------|
| `events.json` | All events (dates, times, recurrence, exceptions) |
| `settings.json` | User preferences, colors, keybindings, and last cursor position |

Writes are atomic (write-to-temp-then-rename) for crash safety. Cursor position is restored on next startup.

## External Editor

Press `e` on any event to open it in `$EDITOR` (or `vi`). The editor shows a structured file with Title, Desc, Start, End, Repeat, Until fields, followed by a `---` separator and freeform notes. Changes are parsed back on save.

The generated template also places notes in a more convenient editing position.

## Requirements

- Terminal with 256-color or truecolor support
- Minimum terminal size: 80x24
- Go 1.25+ (build only)
