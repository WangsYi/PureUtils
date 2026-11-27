#!/usr/bin/env bash
set -euo pipefail

# Reproducible Windows x64 pureimage_codecs.dll cross-build script.
# Intended for Ubuntu (e.g. ubuntu-latest) with x86_64-w64-mingw32 (-posix thread model).
# Goal: static-linked decode-only image codecs (libjpeg-turbo, libwebp, libavif+dav1d,
#       libheif+libde265, libjxl+hwy+brotli+skcms), zero non-Windows runtime DLLs.

HOST="${HOST:-x86_64-w64-mingw32}"
WORK_DIR="${WORK_DIR:-/tmp/pureimage-codecs}"
SRC_DIR="$WORK_DIR/src"
BUILD_DIR="$WORK_DIR/build"
PREFIX="$WORK_DIR/prefix"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${OUT_DIR:-$PWD/dist}"
NPROC="${NPROC:-$(nproc)}"

LIBJPEG_TURBO_VER="3.2.1"
LIBWEBP_VER="v1.6.0"
DAV1D_VER="1.5.1"
LIBAVIF_VER="v1.4.2"
LIBDE265_VER="v1.1.3"
LIBHEIF_VER="v1.23.7"
LIBJXL_VER="v0.13.0"

mkdir -p "$SRC_DIR" "$BUILD_DIR" "$PREFIX/lib/pkgconfig" "$PREFIX/include" "$OUT_DIR"

export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig:$PREFIX/share/pkgconfig"
export PKG_CONFIG_PATH=""
export PKG_CONFIG="pkg-config --static"

CMAKE_TOOLCHAIN="$WORK_DIR/mingw64-toolchain.cmake"
cat > "$CMAKE_TOOLCHAIN" <<EOF
set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR AMD64)
set(CMAKE_C_COMPILER ${HOST}-gcc)
set(CMAKE_CXX_COMPILER ${HOST}-g++)
set(CMAKE_RC_COMPILER ${HOST}-windres)
set(CMAKE_AR ${HOST}-ar)
set(CMAKE_RANLIB ${HOST}-ranlib)
set(CMAKE_FIND_ROOT_PATH ${PREFIX})
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)
EOF

MESON_CROSS="$WORK_DIR/mingw64-cross.ini"
cat > "$MESON_CROSS" <<EOF
[binaries]
c = '${HOST}-gcc'
cpp = '${HOST}-g++'
ar = '${HOST}-ar'
strip = '${HOST}-strip'
windres = '${HOST}-windres'
pkg-config = 'pkg-config'

[built-in options]
c_args = ['-Os', '-ffunction-sections', '-fdata-sections', '-DNDEBUG', '-I${PREFIX}/include']
cpp_args = ['-Os', '-ffunction-sections', '-fdata-sections', '-DNDEBUG', '-I${PREFIX}/include']
c_link_args = ['-L${PREFIX}/lib', '-Wl,--gc-sections']
cpp_link_args = ['-L${PREFIX}/lib', '-Wl,--gc-sections']

[properties]
pkg_config_libdir = ['${PREFIX}/lib/pkgconfig', '${PREFIX}/share/pkgconfig']
needs_exe_wrapper = true

[host_machine]
system = 'windows'
cpu_family = 'x86_64'
cpu = 'x86_64'
endian = 'little'
EOF

fetch_tar() {
  local name="$1"
  local url="$2"
  local dest="$SRC_DIR/$name"
  rm -rf "$dest"
  mkdir -p "$dest"
  echo "==> Downloading $name ($url)"
  curl -fsSL --retry 3 --retry-delay 2 "$url" | tar -xz --strip-components=1 -C "$dest"
}

echo "=== [1/6] Building libjpeg-turbo $LIBJPEG_TURBO_VER ==="
fetch_tar "libjpeg-turbo" "https://github.com/libjpeg-turbo/libjpeg-turbo/archive/refs/tags/${LIBJPEG_TURBO_VER}.tar.gz"
cmake -G Ninja -S "$SRC_DIR/libjpeg-turbo" -B "$BUILD_DIR/libjpeg-turbo" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-O2 -ffunction-sections -fdata-sections -DNDEBUG" \
  -DENABLE_SHARED=OFF \
  -DENABLE_STATIC=ON \
  -DWITH_TURBOJPEG=ON \
  -DWITH_SIMD=ON \
  -DWITH_JPEG8=ON \
  -DWITH_TOOLS=OFF \
  -DWITH_JAVA=OFF
cmake --build "$BUILD_DIR/libjpeg-turbo" -j"$NPROC"
cmake --install "$BUILD_DIR/libjpeg-turbo"

echo "=== [2/6] Building libwebp $LIBWEBP_VER ==="
fetch_tar "libwebp" "https://github.com/webmproject/libwebp/archive/refs/tags/${LIBWEBP_VER}.tar.gz"
cmake -G Ninja -S "$SRC_DIR/libwebp" -B "$BUILD_DIR/libwebp" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG" \
  -DBUILD_SHARED_LIBS=OFF \
  -DWEBP_BUILD_ANIM_UTILS=OFF \
  -DWEBP_BUILD_CWEBP=OFF \
  -DWEBP_BUILD_DWEBP=OFF \
  -DWEBP_BUILD_GIF2WEBP=OFF \
  -DWEBP_BUILD_IMG2WEBP=OFF \
  -DWEBP_BUILD_VWEBP=OFF \
  -DWEBP_BUILD_WEBPINFO=OFF \
  -DWEBP_BUILD_WEBPMUX=OFF \
  -DWEBP_BUILD_EXTRAS=OFF \
  -DWEBP_BUILD_LIBWEBPMUX=OFF \
  -DWEBP_BUILD_LIBWEBPDEMUX=ON
cmake --build "$BUILD_DIR/libwebp" -j"$NPROC"
cmake --install "$BUILD_DIR/libwebp"

echo "=== [3/6] Building dav1d $DAV1D_VER & libavif $LIBAVIF_VER ==="
fetch_tar "dav1d" "https://github.com/videolan/dav1d/archive/refs/tags/${DAV1D_VER}.tar.gz"
meson setup "$BUILD_DIR/dav1d" "$SRC_DIR/dav1d" \
  --cross-file "$MESON_CROSS" \
  --prefix="$PREFIX" \
  --libdir=lib \
  --default-library=static \
  --buildtype=release \
  -Denable_tools=false \
  -Denable_tests=false \
  -Denable_examples=false \
  -Denable_docs=false \
  -Denable_asm=true \
  -Dxxhash_muxer=disabled
ninja -C "$BUILD_DIR/dav1d" -j"$NPROC"
ninja -C "$BUILD_DIR/dav1d" install

fetch_tar "libavif" "https://github.com/AOMediaCodec/libavif/archive/refs/tags/${LIBAVIF_VER}.tar.gz"
cmake -G Ninja -S "$SRC_DIR/libavif" -B "$BUILD_DIR/libavif" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_PREFIX_PATH="$PREFIX" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG" \
  -DBUILD_SHARED_LIBS=OFF \
  -DAVIF_CODEC_DAV1D=SYSTEM \
  -DAVIF_CODEC_AOM=OFF \
  -DAVIF_CODEC_LIBGAV1=OFF \
  -DAVIF_CODEC_RAV1E=OFF \
  -DAVIF_CODEC_SVT=OFF \
  -DAVIF_LIBYUV=OFF \
  -DAVIF_LIBSHARPYUV=OFF \
  -DAVIF_BUILD_APPS=OFF \
  -DAVIF_BUILD_TESTS=OFF \
  -DAVIF_BUILD_EXAMPLES=OFF \
  -DAVIF_BUILD_GDK_PIXBUF=OFF \
  -DAVIF_ENABLE_WERROR=OFF
cmake --build "$BUILD_DIR/libavif" -j"$NPROC"
cmake --install "$BUILD_DIR/libavif"

echo "=== [4/6] Building libde265 $LIBDE265_VER & libheif $LIBHEIF_VER (LGPL-3.0-or-later decode-only) ==="
fetch_tar "libde265" "https://github.com/strukturag/libde265/archive/refs/tags/${LIBDE265_VER}.tar.gz"
cmake -G Ninja -S "$SRC_DIR/libde265" -B "$BUILD_DIR/libde265" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DLIBDE265_STATIC_BUILD" \
  -DCMAKE_CXX_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DLIBDE265_STATIC_BUILD" \
  -DBUILD_SHARED_LIBS=OFF \
  -DENABLE_SDL=OFF \
  -DENABLE_DECODER=OFF \
  -DENABLE_ENCODER=OFF \
  -DENABLE_SHERLOCK265=OFF \
  -DENABLE_SIMD=ON
cmake --build "$BUILD_DIR/libde265" -j"$NPROC"
cmake --install "$BUILD_DIR/libde265"
install -m 644 "$SRC_DIR/libde265/libde265/"*.h "$PREFIX/include/libde265/" || true
if [[ -d "$BUILD_DIR/libde265/libde265" ]]; then
  find "$BUILD_DIR/libde265/libde265" -maxdepth 1 -name '*.h' -exec install -m 644 {} "$PREFIX/include/libde265/" \;
fi

fetch_tar "libheif" "https://github.com/strukturag/libheif/archive/refs/tags/${LIBHEIF_VER}.tar.gz"
cmake -G Ninja -S "$SRC_DIR/libheif" -B "$BUILD_DIR/libheif" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_PREFIX_PATH="$PREFIX" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DLIBDE265_STATIC_BUILD -DLIBHEIF_STATIC_BUILD" \
  -DCMAKE_CXX_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DLIBDE265_STATIC_BUILD -DLIBHEIF_STATIC_BUILD" \
  -DLIBDE265_INCLUDE_DIR="$PREFIX/include" \
  -DLIBDE265_LIBRARY="$PREFIX/lib/libde265.a" \
  -DBUILD_SHARED_LIBS=OFF \
  -DBUILD_TESTING=OFF \
  -DWITH_EXAMPLES=OFF \
  -DWITH_EXAMPLE_HEIF_VIEW=OFF \
  -DWITH_GDK_PIXBUF=OFF \
  -DENABLE_PLUGIN_LOADING=OFF \
  -DWITH_LIBDE265=ON \
  -DWITH_LIBDE265_PLUGIN=OFF \
  -DWITH_X265=OFF \
  -DWITH_AOM_DECODER=OFF \
  -DWITH_AOM_ENCODER=OFF \
  -DWITH_DAV1D=OFF \
  -DWITH_RAV1E=OFF \
  -DWITH_SvtEnc=OFF \
  -DWITH_KVAZAAR=OFF \
  -DWITH_UVG266=OFF \
  -DWITH_VVDEC=OFF \
  -DWITH_VVENC=OFF \
  -DWITH_JPEG_DECODER=OFF \
  -DWITH_JPEG_ENCODER=OFF \
  -DWITH_OpenJPEG_DECODER=OFF \
  -DWITH_OpenJPEG_ENCODER=OFF \
  -DWITH_LIBSHARPYUV=OFF \
  -DWITH_HEADER_COMPRESSION=OFF \
  -DWITH_UNCOMPRESSED_CODEC=OFF
cmake --build "$BUILD_DIR/libheif" -j"$NPROC"
cmake --install "$BUILD_DIR/libheif"

echo "=== [5/6] Building libjxl $LIBJXL_VER (decoder-only + hwy + brotli + skcms) ==="
rm -rf "$SRC_DIR/libjxl"
git clone --depth=1 --branch "$LIBJXL_VER" https://github.com/libjxl/libjxl.git "$SRC_DIR/libjxl"
git -C "$SRC_DIR/libjxl" submodule update --init --depth=1 \
  third_party/brotli third_party/highway third_party/skcms

cmake -G Ninja -S "$SRC_DIR/libjxl" -B "$BUILD_DIR/libjxl" \
  -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_LIBDIR=lib \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_C_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DJXL_STATIC_DEFINE -DJXL_THREADS_STATIC_DEFINE -DJXL_CMS_STATIC_DEFINE" \
  -DCMAKE_CXX_FLAGS_RELEASE="-Os -ffunction-sections -fdata-sections -DNDEBUG -DJXL_STATIC_DEFINE -DJXL_THREADS_STATIC_DEFINE -DJXL_CMS_STATIC_DEFINE" \
  -DBUILD_SHARED_LIBS=OFF \
  -DBUILD_TESTING=OFF \
  -DJPEGXL_STATIC=OFF \
  -DJPEGXL_ENABLE_TOOLS=OFF \
  -DJPEGXL_ENABLE_DEVTOOLS=OFF \
  -DJPEGXL_ENABLE_DOXYGEN=OFF \
  -DJPEGXL_ENABLE_MANPAGES=OFF \
  -DJPEGXL_ENABLE_BENCHMARK=OFF \
  -DJPEGXL_ENABLE_EXAMPLES=OFF \
  -DJPEGXL_ENABLE_JNI=OFF \
  -DJPEGXL_ENABLE_SJPEG=OFF \
  -DJPEGXL_ENABLE_OPENEXR=OFF \
  -DJPEGXL_ENABLE_TCMALLOC=OFF \
  -DJPEGXL_ENABLE_PLUGINS=OFF \
  -DJPEGXL_ENABLE_VIEWERS=OFF \
  -DJPEGXL_ENABLE_SKCMS=ON \
  -DJPEGXL_ENABLE_TRANSCODE_JPEG=ON \
  -DJPEGXL_BUNDLE_LIBPNG=OFF \
  -DJPEGXL_ENABLE_WASM_THREADS=OFF
cmake --build "$BUILD_DIR/libjxl" --target jxl_dec jxl_threads jxl_cms hwy brotlidec brotlicommon -j"$NPROC"

mkdir -p "$PREFIX/include/jxl"
cp -f "$SRC_DIR/libjxl/lib/include/jxl/"*.h "$PREFIX/include/jxl/"
cp -f "$BUILD_DIR/libjxl/lib/include/jxl/"*.h "$PREFIX/include/jxl/"
for libname in libjxl_dec.a libjxl_threads.a libjxl_cms.a libhwy.a libbrotlidec.a libbrotlicommon.a; do
  FOUND_LIB="$(find "$BUILD_DIR/libjxl" -name "$libname" | head -n 1)"
  if [[ -z "$FOUND_LIB" || ! -f "$FOUND_LIB" ]]; then
    echo "ERROR: Missing built archive $libname" >&2
    exit 1
  fi
  install -m 644 "$FOUND_LIB" "$PREFIX/lib/$libname"
done

# Ensure no stray import libraries exist in staging prefix
find "$PREFIX/lib" -name "*.dll.a" -print -delete || true

echo "=== [6/6] Linking pureimage_codecs.dll & smoke_test.exe ==="
"${HOST}-gcc" -O2 -Wall -Wextra -Werror \
  -ffunction-sections -fdata-sections -DNDEBUG \
  -DLIBHEIF_STATIC_BUILD -DLIBDE265_STATIC_BUILD \
  -DJXL_STATIC_DEFINE -DJXL_THREADS_STATIC_DEFINE -DJXL_CMS_STATIC_DEFINE \
  -I"$PREFIX/include" -I"$SCRIPT_DIR" \
  -c "$SCRIPT_DIR/pureimage_codecs.c" -o "$BUILD_DIR/pureimage_codecs.o"

# Inspect what archives were produced so optional helper libs (e.g. libsharpyuv) are linked if present
EXTRA_WEBP_LIBS=""
if [[ -f "$PREFIX/lib/libsharpyuv.a" ]]; then
  EXTRA_WEBP_LIBS="$PREFIX/lib/libsharpyuv.a"
fi
WEBP_CORE_LIB="$PREFIX/lib/libwebp.a"
if [[ ! -f "$WEBP_CORE_LIB" && -f "$PREFIX/lib/libwebpdecoder.a" ]]; then
  WEBP_CORE_LIB="$PREFIX/lib/libwebpdecoder.a"
fi

STDCPP_A="$("${HOST}-g++" -print-file-name=libstdc++.a)"
WINPTHREAD_A="$("${HOST}-gcc" -print-file-name=libwinpthread.a)"
if [[ ! -f "$STDCPP_A" || ! -f "$WINPTHREAD_A" ]]; then
  echo "ERROR: Missing static runtime archives: STDCPP_A=$STDCPP_A WINPTHREAD_A=$WINPTHREAD_A" >&2
  exit 1
fi
EXTRA_RUNTIME_LIBS=()
ATOMIC_A="$("${HOST}-gcc" -print-file-name=libatomic.a)"
if [[ -f "$ATOMIC_A" ]]; then
  EXTRA_RUNTIME_LIBS+=("$ATOMIC_A")
fi

"${HOST}-gcc" -shared \
  -Wl,--gc-sections -Wl,--exclude-libs,ALL \
  -static -static-libgcc \
  "$BUILD_DIR/pureimage_codecs.o" \
  -Wl,--start-group \
  "$PREFIX/lib/libturbojpeg.a" \
  "$PREFIX/lib/libwebpdemux.a" \
  "$WEBP_CORE_LIB" \
  $EXTRA_WEBP_LIBS \
  "$PREFIX/lib/libavif.a" \
  "$PREFIX/lib/libdav1d.a" \
  "$PREFIX/lib/libheif.a" \
  "$PREFIX/lib/libde265.a" \
  "$PREFIX/lib/libjxl_dec.a" \
  "$PREFIX/lib/libjxl_threads.a" \
  "$PREFIX/lib/libjxl_cms.a" \
  "$PREFIX/lib/libhwy.a" \
  "$PREFIX/lib/libbrotlidec.a" \
  "$PREFIX/lib/libbrotlicommon.a" \
  "$STDCPP_A" \
  "$WINPTHREAD_A" \
  "${EXTRA_RUNTIME_LIBS[@]}" \
  -Wl,--end-group \
  -o "$OUT_DIR/pureimage_codecs.dll"

"${HOST}-strip" --strip-unneeded "$OUT_DIR/pureimage_codecs.dll"

"${HOST}-gcc" -O2 -Wall -Wextra -Werror -static \
  -I"$SCRIPT_DIR" \
  "$SCRIPT_DIR/smoke_test.c" \
  -o "$OUT_DIR/smoke_test.exe"

echo "=== Verifying PE imports (must be Windows system DLLs only) ==="
IMPORTS="$("${HOST}-objdump" -p "$OUT_DIR/pureimage_codecs.dll" | awk '/DLL Name:/ {print tolower($3)}' | sort -u)"
echo "$IMPORTS"

while IFS= read -r dll; do
  [[ -z "$dll" ]] && continue
  case "$dll" in
    advapi32.dll|api-ms-win-*.dll|bcrypt.dll|gdi32.dll|kernel32.dll|msvcrt.dll|ntdll.dll|ole32.dll|rpcrt4.dll|secur32.dll|shell32.dll|shlwapi.dll|ucrtbase.dll|user32.dll|ws2_32.dll)
      ;;
    *)
      echo "ERROR: Non-system DLL dependency detected in pureimage_codecs.dll: $dll" >&2
      exit 1
      ;;
  esac
done <<< "$IMPORTS"

echo "=== Verifying PE exports ==="
EXPORTS="$("${HOST}-objdump" -p "$OUT_DIR/pureimage_codecs.dll")"
REQUIRED_EXPORTS=(
  pic_api_version
  pic_capabilities
  pic_decode_image
  pic_decode_animation
  pic_result_frame_count
  pic_result_orig_width
  pic_result_orig_height
  pic_result_flags
  pic_result_frame_width
  pic_result_frame_height
  pic_result_frame_delay_ms
  pic_result_frame_rgba
  pic_result_exif
  pic_result_error
  pic_result_free
)
for sym in "${REQUIRED_EXPORTS[@]}"; do
  if ! grep -qw "$sym" <<< "$EXPORTS"; then
    echo "ERROR: Required export missing: $sym" >&2
    exit 1
  fi
done
echo "All ${#REQUIRED_EXPORTS[@]} required exports present."

echo "=== Collecting third-party license notices ==="
LICENSE_OUT="$OUT_DIR/LICENSES-pureimage-codecs.txt"
cat > "$LICENSE_OUT" <<EOF
PureImage Native Codecs Bundle (pureimage_codecs.dll)
Third-Party Open-Source Software Notices & Licenses
======================================================

All bundled libraries are licensed under permissive (IJG/BSD/MIT/Apache-2.0)
or GNU LGPL-3.0-or-later licenses, compatible with PureUtils (GNU AGPL-3.0).

Included library versions:
- libjpeg-turbo ${LIBJPEG_TURBO_VER}
- libwebp ${LIBWEBP_VER}
- dav1d ${DAV1D_VER}
- libavif ${LIBAVIF_VER}
- libde265 ${LIBDE265_VER}
- libheif ${LIBHEIF_VER}
- libjxl ${LIBJXL_VER} (+ brotli, highway, skcms)

EOF

append_license() {
  local title="$1"
  local file="$2"
  if [[ -f "$file" ]]; then
    {
      echo "--------------------------------------------------------------------------------"
      echo "[$title] ($file)"
      echo "--------------------------------------------------------------------------------"
      cat "$file"
      echo ""
    } >> "$LICENSE_OUT"
  fi
}

append_license "libjpeg-turbo" "$SRC_DIR/libjpeg-turbo/LICENSE.md"
append_license "libjpeg-turbo README.ijg" "$SRC_DIR/libjpeg-turbo/README.ijg"
append_license "libwebp" "$SRC_DIR/libwebp/COPYING"
append_license "dav1d" "$SRC_DIR/dav1d/COPYING"
append_license "libavif" "$SRC_DIR/libavif/LICENSE"
append_license "libde265" "$SRC_DIR/libde265/COPYING"
append_license "libheif" "$SRC_DIR/libheif/COPYING"
append_license "libjxl" "$SRC_DIR/libjxl/LICENSE"
append_license "brotli" "$SRC_DIR/libjxl/third_party/brotli/LICENSE"
append_license "highway" "$SRC_DIR/libjxl/third_party/highway/LICENSE"
append_license "skcms" "$SRC_DIR/libjxl/third_party/skcms/LICENSE"

"${HOST}-size" -A "$OUT_DIR/pureimage_codecs.dll"
ls -lh "$OUT_DIR/pureimage_codecs.dll" "$OUT_DIR/smoke_test.exe"

7z a -t7z -mx=9 "$OUT_DIR/pureimage-codecs-x64.7z" \
  "$OUT_DIR/pureimage_codecs.dll" \
  "$OUT_DIR/LICENSES-pureimage-codecs.txt" >/dev/null

(
  cd "$OUT_DIR"
  sha256sum pureimage_codecs.dll pureimage-codecs-x64.7z LICENSES-pureimage-codecs.txt > SHA256SUMS.txt
)
cat "$OUT_DIR/SHA256SUMS.txt"
ls -lh "$OUT_DIR"
