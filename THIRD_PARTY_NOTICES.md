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
| [`github.com/kettek/apng`](https://github.com/kettek/apng) | Animated PNG decoder | MIT |
| [`github.com/spf13/afero`](https://github.com/spf13/afero) | Filesystem abstraction | Apache-2.0 |
| [`github.com/dustin/go-humanize`](https://github.com/dustin/go-humanize) | Human-readable sizes & formatting | MIT |
| [`github.com/google/uuid`](https://github.com/google/uuid) | UUID generation | BSD-3-Clause |

## Prebuilt Native Libraries / 内置可选原生动态库

- `pureimage_codecs.dll` (bundled when **PureImage** is installed, fetched into `libs/codecs/pureimage_codecs.dll`) — Unified Windows x64 decode-only native library loaded cleanly at runtime via pure-Go `syscall.LazyDLL` (`CGO_ENABLED=0`) for scaled JPEG and native SIMD decoding of WebP, AVIF, HEIC, and JPEG XL:
  - Reproducible from source via [`tools/build-pureimage-codecs/build.sh`](tools/build-pureimage-codecs/build.sh) and [`.github/workflows/build-pureimage-codecs.yml`](.github/workflows/build-pureimage-codecs.yml).
  - Statically links decode-only builds of **libjpeg-turbo** `3.2.1` (IJG / BSD-3-Clause / zlib), **libwebp** `v1.6.0` (BSD-3-Clause), **libavif** `v1.4.2` + **dav1d** `1.5.1` (BSD-2-Clause), **libheif** `v1.23.7` + **libde265** `v1.1.3` (GNU LGPL-3.0-or-later, zero GPL encoders/plugins), and **libjxl** `v0.13.0` + **highway** / **brotli** / **skcms** (BSD-3-Clause / Apache-2.0 / MIT), all compatible with PureUtils' GNU AGPL-3.0 release. Full upstream license texts are distributed alongside in `LICENSES-pureimage-codecs.txt`.

- `libmpv-2.dll` (bundled when **PureVideo** is installed, fetched into `libs/mpv/libmpv-2.dll`) — **mpv / libmpv** (<https://mpv.io>) built in a Tier 1 trimmed Windows x64 configuration with static **FFmpeg**, **libass**, **libplacebo**, **dav1d**, and companion dependencies.
  - Used at runtime by **PureVideo** via pure-Go dynamic loading (`syscall.LoadLibrary`) for hardware-accelerated video playback, subtitles, and container demuxing.
  - Reproducible from source via [`tools/build-libmpv/build.sh`](tools/build-libmpv/build.sh) and [`.github/workflows/build-libmpv.yml`](.github/workflows/build-libmpv.yml).
  - Subject to the upstream **GPL-2.0-or-later / LGPL-2.1-or-later / MIT / BSD / ISC** licenses of mpv and its statically linked media codecs (compatible with PureUtils' GNU AGPL-3.0 release).

Full dependency graph can always be inspected via `go list -m all`.
