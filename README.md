# Timelapse-PixelBattle

Simple tool to render a timelapse / pixel battle video or single photo from pixel placement records.
---

## Prerequisites

* Go 1.20+ (or compatible)
* `ffmpeg` binary installed and available on `PATH` (used by `github.com/u2takey/ffmpeg-go`)
* assets folder with Minecraft textures in .png format (not included due to licensing; obtain from your own Minecraft installation or resource packs)
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
CGO_ENABLED=1 go build -o timelapse ./cmd/timelapse-pb # if windows go build -o timelapse.exe
```

**Recommended build flags:**
```bash
CGO_ENABLED=1 GOAMD64=v3 go build -o timelapse -ldflags="-s -w" -trimpath -tags netgo ./cmd/timelapse-pb # if windows go build -o timelapse.exe
```


This produces an executable named `timelapse` (or `timelapse.exe` on Windows).

---

## Usage

The program uses flags and command to set itself up and run. Minimal required flags are: filename, db-* (base on your setup).

The commands available: render, photo

**Example:**
```
./timelapse render --filename=out.mp4 --db-source=./some.db [options]
./timelapse photo --filename=out.png --db-source=./some.db [options]
```

Flags:

* `--width` (int) — canvas width (default `1080`)
* `--height` (int) — canvas height (default `1920`)
* `--iterations` (int) — actions per frame (default `16`)
* `--texture-size` (int) — texture size in pixels (default `16`)
* `--framerate` (int) — video framerate (default `24`)
* `--filename` (string) — output filename (required)
* `--local` (bool) — enable local mode database
* `--photo` (bool) — generate single photo instead of video (specify in --filename=FILENAME.png)
* `--debug` (bool) — enable debug mode
* `--playername` (string) — name of player by which the application will filter the data

Database connection flags:

* `--db-ip` (string) — host:port for DB (for local not needed)
* `--db-user` (string) — database user (for local not needed)
* `--db-password` (string) — database password (for local not needed)
* `--db-name` (string) — database name (for local not needed)
* `--db-source` (string) — path to `*.db` file of your local database, used in **local** mode
* `--db-table` (string) — table name

---

## Examples

### Local mode (SQLite database)

The SQL schema should look like this:

```
create table new_co_block
(
    id        INTEGER
        primary key,
    timestamp TIMESTAMP,
    owner     TEXT,
    x         BIGINT,
    y         BIGINT,
    c         TEXT
);

create index idx_owner_id
    on new_co_block (owner, id);
```

**If it's not containing indexes or id (PRIMARY KEY), the performance may degrade/code may fail.**

Run:

```bash
./timelapse render --local --db-source=dump.sql --db-table=TaBLe --filename=timelapse.mp4 --width=1080 --height=1920 --framerate=30 --iterations=16
```

For a single photo from the dump:

```bash
./timelapse photo --local --db-source=dump.sql --db-table=TaBLe --filename=timelapse.mp4 --width=1080 --height=1920 --framerate=30 --iterations=16
```

### Normal mode (db)

Ensure your DB is reachable and contains table `PB` with compatible columns. Example run:

```bash
./timelapse render --db-ip=127.0.0.1:9000 --db-table=TaBLe --db-user=user --db-password=pass --db-name=default --filename=timelapse.mp4
```

---

## Troubleshooting

* If ffmpeg errors appear, verify `ffmpeg` is installed and in `PATH`.
* If reading a local SQLite fails, ensure that you compiled it with CGO enabled. Otherwise, program will fail.
* For large datasets (tested on 2.3mil - 500-600MB ram usage) around 10 mil I recommend a machine with at least 4GB RAM free.

---

## Development notes

* The program uses `graphics.EncodeGPU` and `graphics.GeneratePhotoLocal` to produce output. Adjust `texture-size`, `iterations`, `width` and `height` to tune performance and visual quality.
* `ffmpeg-go` is used to assemble encoded frames into the final video; the package toggles off compiled command logging by default.
* For anyone who tried and got *problems* I recommend to open GH issue and we can solve this out. The plugin for this program will be released shortly.
