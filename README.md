# Timelapse-PixelBattle

Simple tool to render a timelapse / pixel battle video or single photo from pixel placement records. Supports two modes:

* **local mode**: load an SQL dump file (expected `INSERT INTO default.PB ...` lines) and render from it.
* **normal mode**: connect to a database and read records in batches (table `PB` expected).

---

## Prerequisites

* Go 1.20+ (or compatible)
* `ffmpeg` binary installed and available on `PATH` (used by `github.com/u2takey/ffmpeg-go`)
* A shell/terminal

Install ffmpeg examples:

* Debian/Ubuntu: `sudo apt update && sudo apt install ffmpeg`
* macOS (Homebrew): `brew install ffmpeg`
* Windows (chocolatey): `choco install ffmpeg`

---

## Build

From project root:

```bash
# download dependencies and build
go mod tidy
go build -o timelapse # if windows go build -o timelapse.exe
```
This produces an executable named `timelapse` (or `timelapse.exe` on Windows).

---

## Usage

The program uses flags. Minimal required flag: `--filename` (output file). // but output will be nothing. literally. see below

```
./timelapse --filename=out.mp4 [options]
```

Flags:

* `--width` (int) — canvas width (default `1080`)
* `--height` (int) — canvas height (default `1920`)
* `--iterations` (int) — actions per frame (default `16`)
* `--texture-size` (int) — texture size in pixels (default `16`)
* `--framerate` (int) — video framerate (default `24`)
* `--filename` (string) — output filename (required)
* `--local-mode` (string) — path to `.sql` file to load (activates *local mode*)
* `--photo` — generate single photo instead of video (specify in --filename=FILENAME.png)
* `--debug` — enable debug mode

Database connection flags (used in *normal mode*):

* `--database-ip` (string) — host:port for DB
* `--database-user` (string)
* `--database-password` (string)
* `--database-name` (string)

---

## Examples

### Local mode (SQL dump)

The SQL dump should contain lines like:

```
INSERT INTO default.PB timestamp x y c VALUES ('2025-01-01T12:00:00.000000' 12 34 'STONE');
```

Run:

```bash
./timelapse --local-mode=dump.sql --filename=timelapse.mp4 --width=1080 --height=1920 --framerate=30 --iterations=16
```

For a single photo from the dump:

```bash
./timelapse --local-mode=dump.sql --filename=photo.png --photo
```

### Normal mode (database)

Ensure your DB is reachable and contains table `PB` with compatible columns. Example run:

```bash
./timelapse --database-ip=127.0.0.1:9000 --database-user=user --database-password=pass --database-name=default --filename=timelapse.mp4
```

Notes:

* The code loads records in batches of 1000 and will re-check the max record count while running (so new records can be picked up).
* The normal mode currently assumes table name `PB` and a specific schema used by the Minecraft plugin that produces the insert lines.

---

## Troubleshooting

* If ffmpeg errors appear, verify `ffmpeg` is installed and in `PATH`.
* If reading a local SQL file fails, ensure the file encoding is UTF-8 and the `INSERT` lines match the expected pattern. Local parsing strips parentheses, quotes and commas and expects values: `timestamp x y c`.
* For large datasets, ensure sufficient RAM and disk space. The program triggers `runtime.GC()` during batch processing but may still use significant memory.

---

## Development notes

* The program uses `graphics.EncodeGPU` and `graphics.GeneratePhotoLocal` to produce output. Adjust `texture-size`, `iterations`, `width` and `height` to tune performance and visual quality.
* `ffmpeg-go` is used to assemble encoded frames into the final video; the package toggles off compiled command logging by default.
