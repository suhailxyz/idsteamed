# idsteamed

Automates the process of looking up Steam game IDs, and creating individual files for each game. This is used for adding games from GameHub Lite to emulation frontends like ES-DE and Beacon. The tool processes a text file with Steam game names and generates individual `.steam` files with game IDs.

**Important:** Copy paste game names directly from the Steam store page. (See end for shortcut)

## Getting Started

**Easiest way:** Use the pre-built binary in `bin/` directory. No installation needed.

```bash
chmod +x idsteamed
./idsteamed games.txt
```

Or if building from source, the binary will be in the `bin/` directory:

```bash
go build -o bin/idsteamed main.go
./bin/idsteamed games.txt
```

## Usage

```bash
./idsteamed [flags] <input_file.txt>
```

### Flags (Optional)

- `--output <dir>` - Output directory (default: "output")
- `--workers <n>` - Number of concurrent workers (default: 8)
- `--skip-existing` - Skip games that already have .steam files
- `--verbose` - Show detailed output

### Examples

```bash
./idsteamed games.txt
./idsteamed --output my_games --workers 16 games.txt
./idsteamed --skip-existing --verbose games.txt
```

## Input Format

One game name per line. See `examples/sample_games.txt` for an example:

```
Cuphead
The Witcher 3: Wild Hunt
Portal
Half-Life 2
```

## Output

Creates `.steam` files in the output directory. Each file contains a single line with the Steam game ID.


## Building from Source

If you want to build it yourself:

```bash
go build -o idsteamed main.go
```

Cross-platform builds:

```bash
GOOS=darwin GOARCH=arm64 go build -o idsteamed main.go    # Mac
GOOS=linux GOARCH=amd64 go build -o idsteamed main.go     # Linux
GOOS=windows GOARCH=amd64 go build -o idsteamed.exe main.go  # Windows
```
## Extracting Game Titles from Screenshots

If you have screenshots of your game library (e.g., from GameHub Lite), you can use AI to process them faster:

1. Copy the prompt from `prompt.md`
2. Paste it into ChatGPT (or any AI chat)
3. Upload your screenshot(s)
4. The AI will return a list of game titles in a plaintext codeblock
5. Save that list to a `.txt` file and use it with this tool

The prompt handles truncated titles (like "Disco Elysium - T..") by looking up the complete Steam title, and automatically deduplicates games across multiple screenshots.
