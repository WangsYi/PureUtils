# Third-Party Notices / 第三方开源组件声明

PureUtils is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)** — see [LICENSE](LICENSE).
PureUtils builds upon and bundles the following third-party open-source packages and libraries:

## Go Dependencies (declared in `go.mod`)

| Package | Purpose | License |
|---|---|---|
| [`github.com/egoist/mygo`](https://github.com/egoist/mygo) | Native GPU-rendered desktop UI framework (Windows Direct2D/DX) | MIT |
| [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | Pure-Go SQLite database driver (cgo-free) | BSD-3-Clause |
| [`golang.org/x/image`](https://pkg.go.dev/golang.org/x/image) | Image codecs & resampling algorithms | BSD-3-Clause |
| [`golang.org/x/sys`](https://pkg.go.dev/golang.org/x/sys) | Windows OS / Win32 API bindings | BSD-3-Clause |
| [`github.com/fogleman/gg`](https://github.com/fogleman/gg) | 2D vector drawing & annotation baking | MIT |
| [`github.com/go-ole/go-ole`](https://github.com/go-ole/go-ole) | Windows COM bindings | MIT |
| [`github.com/bodgit/sevenzip`](https://github.com/bodgit/sevenzip) | 7z archive reader | BSD-3-Clause |
| [`github.com/nwaples/rardecode/v2`](https://github.com/nwaples/rardecode) | RAR v4/v5 archive reader | BSD-2-Clause |
| [`github.com/klauspost/compress`](https://github.com/klauspost/compress) | Deflate, Zstandard, Gzip compression | BSD-3-Clause / Apache-2.0 |
| [`github.com/ulikunitz/xz`](https://github.com/ulikunitz/xz) | XZ/LZMA2 compression & decompression | BSD-3-Clause |
| [`github.com/andybalholm/brotli`](https://github.com/andybalholm/brotli) | Brotli compression | MIT |
| [`github.com/pierrec/lz4/v4`](https://github.com/pierrec/lz4) | LZ4 compression | BSD-3-Clause |
| [`github.com/gen2brain/avif`](https://github.com/gen2brain/avif) | AVIF image decoder (WASM via wazero) | MIT / BSD-3-Clause |
| [`github.com/gen2brain/heic`](https://github.com/gen2brain/heic) | HEIC/HEIF image decoder | MIT / LGPL / BSD |
| [`github.com/gen2brain/jpegxl`](https://github.com/gen2brain/jpegxl) | JPEG XL decoder | BSD-3-Clause |
| [`github.com/gen2brain/webp`](https://github.com/gen2brain/webp) | WebP decoder | BSD-3-Clause |
| [`github.com/kettek/apng`](https://github.com/kettek/apng) | Animated PNG decoder | MIT |
| [`github.com/tetratelabs/wazero`](https://github.com/tetratelabs/wazero) | Zero-dependency WebAssembly runtime | Apache-2.0 |
| [`github.com/spf13/afero`](https://github.com/spf13/afero) | Filesystem abstraction | Apache-2.0 |
| [`github.com/dustin/go-humanize`](https://github.com/dustin/go-humanize) | Human-readable sizes & formatting | MIT |
| [`github.com/google/uuid`](https://github.com/google/uuid) | UUID generation | BSD-3-Clause |

## Optional Prebuilt Native Library

- `libs/turbojpeg/turbojpeg.dll` — **libjpeg-turbo** (<https://libjpeg-turbo.org>), optionally used at runtime by **PureImage** via pure-Go dynamic loading (`purego` / `syscall.LazyDLL`) for hardware-fast 1/2, 1/4, 1/8 DCT scaling of large JPEGs.
  - Licensed under the **IJG (Independent JPEG Group) License**, the **Modified 3-clause BSD License**, and the **zlib License**.
  - Users can omit `turbojpeg.dll` completely via the installer component checkbox or compile with `-CoreOnly` / `-tags nofreeimage`.

Full dependency graph can always be inspected via `go list -m all`.
