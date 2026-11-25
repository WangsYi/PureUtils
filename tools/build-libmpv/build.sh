#!/usr/bin/env bash
set -euo pipefail

# Reproducible Tier 1 trimmed Windows x64 libmpv-2.dll cross-build script.
# Intended for Ubuntu (e.g. ubuntu-latest) with x86_64-w64-mingw32 (-win32 thread model).
# Goal: local playback only, static-linked dependencies, zero non-Windows runtime DLLs.

HOST="${HOST:-x86_64-w64-mingw32}"
WORK_DIR="${WORK_DIR:-/tmp/mpv-tier1}"
SRC_DIR="$WORK_DIR/src"
BUILD_DIR="$WORK_DIR/build"
PREFIX="$WORK_DIR/prefix"
OUT_DIR="${OUT_DIR:-$PWD/out}"
NPROC="${NPROC:-$(nproc)}"

ZLIB_VER="1.3.1"
WINICONV_VER="0.0.10"
FREETYPE_VER="VER-2-13-3"
FRIBIDI_VER="1.0.16"
HARFBUZZ_VER="10.4.0"
LIBUNIBREAK_VER="6.1"
LIBASS_VER="0.17.3"
DAV1D_VER="1.5.1"
FFMPEG_VER="n7.1.1"
LIBPLACEBO_VER="v7.351.0"
MPV_VER="v0.40.0"

mkdir -p "$SRC_DIR" "$BUILD_DIR" "$PREFIX/lib/pkgconfig" "$PREFIX/include" "$OUT_DIR"

export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig:$PREFIX/share/pkgconfig"
export PKG_CONFIG_PATH=""
export PKG_CONFIG="pkg-config --static"
export CFLAGS="-Os -ffunction-sections -fdata-sections -DNDEBUG"
export CXXFLAGS="-Os -ffunction-sections -fdata-sections -DNDEBUG"
export LDFLAGS="-L$PREFIX/lib -Wl,--gc-sections -static-libgcc -static-libstdc++"

CROSS_FILE="$WORK_DIR/mingw64-cross.ini"
cat > "$CROSS_FILE" <<EOF
[binaries]
c = '${HOST}-gcc'
cpp = '${HOST}-g++'
ar = '${HOST}-ar'
strip = '${HOST}-strip'
windres = '${HOST}-windres'
dlltool = '${HOST}-dlltool'
pkgconfig = 'pkg-config'

[built-in options]
c_args = ['-Os', '-ffunction-sections', '-fdata-sections', '-DNDEBUG', '-I${PREFIX}/include']
cpp_args = ['-Os', '-ffunction-sections', '-fdata-sections', '-DNDEBUG', '-I${PREFIX}/include']
c_link_args = ['-L${PREFIX}/lib', '-Wl,--gc-sections', '-static-libgcc', '-static-libstdc++']
cpp_link_args = ['-L${PREFIX}/lib', '-Wl,--gc-sections', '-static-libgcc', '-static-libstdc++']

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

echo "=== [1/10] Building zlib $ZLIB_VER ==="
fetch_tar "zlib" "https://github.com/madler/zlib/archive/refs/tags/v${ZLIB_VER}.tar.gz"
make -C "$SRC_DIR/zlib" -f win32/Makefile.gcc \
  PREFIX="${HOST}-" \
  LOC="-Os -ffunction-sections -fdata-sections -DNDEBUG" \
  -j"$NPROC" libz.a
install -m 644 "$SRC_DIR/zlib/libz.a" "$PREFIX/lib/libz.a"
install -m 644 "$SRC_DIR/zlib/zlib.h" "$SRC_DIR/zlib/zconf.h" "$PREFIX/include/"
cat > "$PREFIX/lib/pkgconfig/zlib.pc" <<EOF
prefix=${PREFIX}
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: zlib
Description: zlib compression library
Version: ${ZLIB_VER}
Libs: -L\${libdir} -lz
Cflags: -I\${includedir}
EOF

echo "=== [2/10] Building win-iconv $WINICONV_VER ==="
fetch_tar "win-iconv" "https://github.com/win-iconv/win-iconv/archive/refs/tags/v${WINICONV_VER}.tar.gz"
"${HOST}-gcc" -Os -ffunction-sections -fdata-sections -DNDEBUG -c \
  "$SRC_DIR/win-iconv/win_iconv.c" -o "$BUILD_DIR/win_iconv.o"
"${HOST}-ar" rcs "$PREFIX/lib/libiconv.a" "$BUILD_DIR/win_iconv.o"
install -m 644 "$SRC_DIR/win-iconv/iconv.h" "$PREFIX/include/iconv.h"
cat > "$PREFIX/lib/pkgconfig/iconv.pc" <<EOF
prefix=${PREFIX}
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: iconv
Description: win-iconv static library
Version: ${WINICONV_VER}
Libs: -L\${libdir} -liconv
Cflags: -I\${includedir}
EOF

echo "=== [3/10] Building freetype $FREETYPE_VER ==="
fetch_tar "freetype" "https://github.com/freetype/freetype/archive/refs/tags/${FREETYPE_VER}.tar.gz"
meson setup "$BUILD_DIR/freetype" "$SRC_DIR/freetype" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=minsize \
  -Dzlib=enabled \
  -Dbzip2=disabled \
  -Dpng=disabled \
  -Dharfbuzz=disabled \
  -Dbrotli=disabled \
  -Dmmap=disabled \
  -Dtests=disabled
ninja -C "$BUILD_DIR/freetype" -j"$NPROC"
ninja -C "$BUILD_DIR/freetype" install

echo "=== [4/10] Building fribidi $FRIBIDI_VER ==="
fetch_tar "fribidi" "https://github.com/fribidi/fribidi/archive/refs/tags/v${FRIBIDI_VER}.tar.gz"
meson setup "$BUILD_DIR/fribidi" "$SRC_DIR/fribidi" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=minsize \
  -Ddocs=false \
  -Dbin=false \
  -Dtests=false
ninja -C "$BUILD_DIR/fribidi" -j"$NPROC"
ninja -C "$BUILD_DIR/fribidi" install
if ! grep -q "FRIBIDI_LIB_STATIC" "$PREFIX/lib/pkgconfig/fribidi.pc"; then
  sed -i 's/^Cflags:/Cflags: -DFRIBIDI_LIB_STATIC/' "$PREFIX/lib/pkgconfig/fribidi.pc"
fi

echo "=== [5/10] Building harfbuzz $HARFBUZZ_VER ==="
fetch_tar "harfbuzz" "https://github.com/harfbuzz/harfbuzz/archive/refs/tags/${HARFBUZZ_VER}.tar.gz"
meson setup "$BUILD_DIR/harfbuzz" "$SRC_DIR/harfbuzz" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=minsize \
  -Dfreetype=enabled \
  -Dglib=disabled \
  -Dgobject=disabled \
  -Dcairo=disabled \
  -Dchafa=disabled \
  -Dicu=disabled \
  -Dgraphite=disabled \
  -Dgraphite2=disabled \
  -Dgdi=disabled \
  -Ddirectwrite=disabled \
  -Dtests=disabled \
  -Ddocs=disabled \
  -Dutilities=disabled \
  -Dintrospection=disabled
ninja -C "$BUILD_DIR/harfbuzz" -j"$NPROC"
ninja -C "$BUILD_DIR/harfbuzz" install

echo "=== [6/10] Building libunibreak $LIBUNIBREAK_VER ==="
fetch_tar "libunibreak" "https://github.com/adah1972/libunibreak/releases/download/libunibreak_6_1/libunibreak-${LIBUNIBREAK_VER}.tar.gz"
mkdir -p "$BUILD_DIR/libunibreak"
pushd "$BUILD_DIR/libunibreak" >/dev/null
"$SRC_DIR/libunibreak/configure" \
  --host="$HOST" \
  --prefix="$PREFIX" \
  --enable-static \
  --disable-shared \
  CFLAGS="-Os -ffunction-sections -fdata-sections -DNDEBUG"
make -j"$NPROC"
make install
popd >/dev/null

echo "=== [7/10] Building libass $LIBASS_VER (DirectWrite, no fontconfig) ==="
fetch_tar "libass" "https://github.com/libass/libass/archive/refs/tags/${LIBASS_VER}.tar.gz"
meson setup "$BUILD_DIR/libass" "$SRC_DIR/libass" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=minsize \
  -Dfontconfig=disabled \
  -Ddirectwrite=enabled \
  -Dcoretext=disabled \
  -Dlibunibreak=enabled \
  -Dasm=enabled \
  -Dtest=disabled \
  -Dcompare=disabled \
  -Dprofile=disabled \
  -Dfuzz=disabled \
  -Dcheckasm=disabled
ninja -C "$BUILD_DIR/libass" -j"$NPROC"
ninja -C "$BUILD_DIR/libass" install

echo "=== [8/10] Building dav1d $DAV1D_VER ==="
fetch_tar "dav1d" "https://github.com/videolan/dav1d/archive/refs/tags/${DAV1D_VER}.tar.gz"
meson setup "$BUILD_DIR/dav1d" "$SRC_DIR/dav1d" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=release \
  -Denable_tools=false \
  -Denable_tests=false \
  -Denable_examples=false \
  -Denable_docs=false \
  -Denable_asm=true
ninja -C "$BUILD_DIR/dav1d" -j"$NPROC"
ninja -C "$BUILD_DIR/dav1d" install

echo "=== [9/10] Building FFmpeg $FFMPEG_VER (Tier 1 local playback trimmed) ==="
fetch_tar "ffmpeg" "https://github.com/FFmpeg/FFmpeg/archive/refs/tags/${FFMPEG_VER}.tar.gz"
mkdir -p "$BUILD_DIR/ffmpeg"
pushd "$BUILD_DIR/ffmpeg" >/dev/null
"$SRC_DIR/ffmpeg/configure" \
  --arch=x86_64 \
  --target-os=mingw32 \
  --cross-prefix="${HOST}-" \
  --pkg-config=pkg-config \
  --pkg-config-flags="--static" \
  --prefix="$PREFIX" \
  --enable-static \
  --disable-shared \
  --enable-gpl \
  --enable-version3 \
  --enable-small \
  --disable-all \
  --disable-autodetect \
  --disable-programs \
  --disable-doc \
  --disable-debug \
  --disable-avdevice \
  --disable-network \
  --disable-postproc \
  --disable-encoders \
  --disable-muxers \
  --disable-devices \
  --disable-pthreads \
  --enable-w32threads \
  --enable-avcodec \
  --enable-avformat \
  --enable-avutil \
  --enable-swscale \
  --enable-swresample \
  --enable-avfilter \
  --enable-zlib \
  --enable-libdav1d \
  --enable-d3d11va \
  --enable-dxva2 \
  --extra-cflags="-I${PREFIX}/include -ffunction-sections -fdata-sections -DNDEBUG" \
  --extra-ldflags="-L${PREFIX}/lib" \
  --enable-protocol=file,pipe,data \
  --enable-demuxer=mov,matroska,avi,asf,asf_o,mpegts,mpegps,mpegvideo,flv,live_flv,rm,ogg,mxf,nut,ivf,yuv4mpegpipe,gif,image2,image2pipe,mp3,aac,flac,wav,aiff,ape,wv,tta,dts,dtshd,ac3,eac3,truehd,amr,ass,srt,webvtt,subviewer,subviewer1,microdvd,mpl2,realtext,sami,stl,vplayer,vobsub,sup,lrc \
  --enable-decoder=h264,hevc,vp8,vp9,libdav1d,av1,mpeg1video,mpeg2video,mpeg4,msmpeg4v1,msmpeg4v2,msmpeg4v3,wmv1,wmv2,wmv3,vc1,flv,h263,h263i,h263p,rv10,rv20,rv30,rv40,theora,vp6,vp6a,vp6f,mjpeg,mjpegb,png,gif,webp,bmp,tiff,qtrle,prores,dnxhd,cinepak,svq1,svq3,rawvideo,v210,utvideo,huffyuv,ffv1,dvvideo,aac,aac_latm,mp1,mp1float,mp2,mp2float,mp3,mp3float,opus,vorbis,flac,alac,ac3,eac3,dca,truehd,mlp,wmav1,wmav2,wmapro,wmavoice,wmalossless,cook,ra_144,ra_288,sipr,ralf,ape,wavpack,amrnb,amrwb,nellymoser,speex,adpcm_ms,adpcm_ima_wav,adpcm_ima_qt,adpcm_swf,adpcm_adx,adpcm_yamaha,pcm_s16le,pcm_s16be,pcm_s24le,pcm_s24be,pcm_s32le,pcm_s32be,pcm_u8,pcm_s8,pcm_f32le,pcm_f32be,pcm_f64le,pcm_f64be,pcm_alaw,pcm_mulaw,pcm_dvd,pcm_bluray,pcm_s16le_planar,pcm_s24daud,ass,ssa,subrip,srt,webvtt,movtext,text,dvbsub,dvdsub,pgssub,xsub,jacosub,microdvd,mpl2,realtext,sami,stl,subviewer,subviewer1,vplayer,ccaption \
  --enable-parser=h264,hevc,av1,vp8,vp9,mpeg4video,mpegvideo,mpegaudio,aac,aac_latm,ac3,dca,flac,opus,vorbis,vc1,mjpeg,png,rv30,rv40,h263,mlp,dvbsub,dvdsub,cook,sipr,bmp,webp,gif \
  --enable-bsf=h264_mp4toannexb,hevc_mp4toannexb,aac_adtstoasc,extract_extradata,av1_frame_split,av1_metadata,vp9_superframe,vp9_superframe_split,null \
  --enable-hwaccel=h264_d3d11va,h264_d3d11va2,hevc_d3d11va,hevc_d3d11va2,vp9_d3d11va,vp9_d3d11va2,av1_d3d11va,av1_d3d11va2,mpeg2_d3d11va,mpeg2_d3d11va2,vc1_d3d11va,vc1_d3d11va2,wmv3_d3d11va,wmv3_d3d11va2,h264_dxva2,hevc_dxva2,vp9_dxva2,av1_dxva2,mpeg2_dxva2,vc1_dxva2,wmv3_dxva2 \
  --enable-filter=buffer,buffersink,abuffer,abuffersink,format,aformat,scale,aresample,null,anull,setpts,asetpts,yadif,bwdif,hwupload,hwdownload,transpose,hflip,vflip,crop,eq,volume,atempo,pad,fps
make -j"$NPROC"
make install
popd >/dev/null

echo "=== [10/10] Building libplacebo $LIBPLACEBO_VER & libmpv $MPV_VER ==="
rm -rf "$SRC_DIR/libplacebo"
git clone --depth=1 --branch "$LIBPLACEBO_VER" https://github.com/haasn/libplacebo.git "$SRC_DIR/libplacebo"
git -C "$SRC_DIR/libplacebo" submodule update --init \
  3rdparty/glad 3rdparty/jinja 3rdparty/markupsafe 3rdparty/fast_float

meson setup "$BUILD_DIR/libplacebo" "$SRC_DIR/libplacebo" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=static \
  --buildtype=minsize \
  -Dopengl=enabled \
  -Dgl-proc-addr=enabled \
  -Dvulkan=disabled \
  -Dvk-proc-addr=disabled \
  -Dd3d11=disabled \
  -Dshaderc=disabled \
  -Dglslang=disabled \
  -Dlcms=disabled \
  -Ddovi=disabled \
  -Dlibdovi=disabled \
  -Dxxhash=disabled \
  -Dunwind=disabled \
  -Ddemos=false \
  -Dtests=false \
  -Dbench=false \
  -Dfuzz=false
ninja -C "$BUILD_DIR/libplacebo" -j"$NPROC"
ninja -C "$BUILD_DIR/libplacebo" install
if ! grep -q "PL_STATIC" "$PREFIX/lib/pkgconfig/libplacebo.pc"; then
  sed -i 's/^Cflags:/Cflags: -DPL_STATIC/' "$PREFIX/lib/pkgconfig/libplacebo.pc"
fi

# Ensure no accidental shared import libraries exist in our staging prefix
find "$PREFIX/lib" -name "*.dll.a" -print -delete || true

fetch_tar "mpv" "https://github.com/mpv-player/mpv/archive/refs/tags/${MPV_VER}.tar.gz"
meson setup "$BUILD_DIR/mpv" "$SRC_DIR/mpv" \
  --cross-file "$CROSS_FILE" \
  --prefix="$PREFIX" \
  --default-library=shared \
  --buildtype=minsize \
  -Dgpl=true \
  -Dcplayer=false \
  -Dlibmpv=true \
  -Dbuild-date=false \
  -Dtests=false \
  -Dfuzzers=false \
  -Dwin32-threads=enabled \
  -Diconv=enabled \
  -Dzlib=enabled \
  -Dwasapi=enabled \
  -Dgl=enabled \
  -Dgl-win32=enabled \
  -Dgl-dxinterop=enabled \
  -Dgl-dxinterop-d3d9=enabled \
  -Ddirect3d=enabled \
  -Dd3d-hwaccel=enabled \
  -Dd3d9-hwaccel=enabled \
  -Dplain-gl=enabled \
  -Dlua=disabled \
  -Djavascript=disabled \
  -Dvapoursynth=disabled \
  -Dlibbluray=disabled \
  -Ddvdnav=disabled \
  -Dcdda=disabled \
  -Ddvbin=disabled \
  -Dlibarchive=disabled \
  -Dlibavdevice=disabled \
  -Drubberband=disabled \
  -Dzimg=disabled \
  -Dlcms2=disabled \
  -Duchardet=disabled \
  -Djpeg=disabled \
  -Dshaderc=disabled \
  -Dspirv-cross=disabled \
  -Dvulkan=disabled \
  -Dd3d11=disabled \
  -Degl=disabled \
  -Degl-android=disabled \
  -Degl-angle=disabled \
  -Degl-angle-lib=disabled \
  -Degl-angle-win32=disabled \
  -Dwin32-smtc=disabled \
  -Dsdl2=disabled \
  -Dsdl2-gamepad=disabled \
  -Dsdl2-audio=disabled \
  -Dsdl2-video=disabled \
  -Dopenal=disabled \
  -Djack=disabled \
  -Dpulse=disabled \
  -Dpipewire=disabled \
  -Dalsa=disabled \
  -Doss-audio=disabled \
  -Dsndio=disabled \
  -Dcoreaudio=disabled \
  -Daudiounit=disabled \
  -Davfoundation=disabled \
  -Ddrm=disabled \
  -Dgbm=disabled \
  -Dwayland=disabled \
  -Dx11=disabled \
  -Dxv=disabled \
  -Dvaapi=disabled \
  -Dvdpau=disabled \
  -Dcuda-hwaccel=disabled \
  -Dcuda-interop=disabled \
  -Dsixel=disabled \
  -Dcaca=disabled \
  -Dcplugins=disabled \
  -Dmanpage-build=disabled \
  -Dhtml-build=disabled \
  -Dpdf-build=disabled
ninja -C "$BUILD_DIR/mpv" -j"$NPROC"
ninja -C "$BUILD_DIR/mpv" install

DLL_SRC="$(find "$PREFIX/bin" "$BUILD_DIR/mpv" -maxdepth 2 -name 'libmpv*.dll' | head -n 1)"
if [[ -z "$DLL_SRC" || ! -f "$DLL_SRC" ]]; then
  echo "ERROR: libmpv-2.dll not found after build" >&2
  exit 1
fi

cp -f "$DLL_SRC" "$OUT_DIR/libmpv-2.dll"
"${HOST}-strip" --strip-unneeded "$OUT_DIR/libmpv-2.dll"

echo "=== Verifying PE imports ==="
IMPORTS="$("${HOST}-objdump" -p "$OUT_DIR/libmpv-2.dll" | awk '/DLL Name:/ {print tolower($3)}' | sort -u)"
echo "$IMPORTS"

while IFS= read -r dll; do
  [[ -z "$dll" ]] && continue
  case "$dll" in
    advapi32.dll|api-ms-win-*.dll|avrt.dll|bcrypt.dll|cfgmgr32.dll|comctl32.dll|crypt32.dll|d3d11.dll|d3d9.dll|dwmapi.dll|dxgi.dll|dxva2.dll|dwrite.dll|gdi32.dll|imm32.dll|kernel32.dll|ksuser.dll|mfplat.dll|msvcrt.dll|ntdll.dll|ole32.dll|oleaut32.dll|opengl32.dll|powrprof.dll|propsys.dll|rpcrt4.dll|secur32.dll|setupapi.dll|shell32.dll|shlwapi.dll|ucrtbase.dll|user32.dll|uxtheme.dll|version.dll|winmm.dll|ws2_32.dll)
      ;;
    *)
      echo "ERROR: Non-system DLL dependency detected: $dll" >&2
      exit 1
      ;;
  esac
done <<< "$IMPORTS"

echo "=== Verifying PE exports ==="
EXPORTS="$("${HOST}-objdump" -p "$OUT_DIR/libmpv-2.dll")"
REQUIRED_EXPORTS=(
  mpv_client_api_version
  mpv_create
  mpv_initialize
  mpv_terminate_destroy
  mpv_set_option_string
  mpv_command
  mpv_set_property
  mpv_get_property
  mpv_wait_event
  mpv_wakeup
  mpv_error_string
)
for sym in "${REQUIRED_EXPORTS[@]}"; do
  if ! grep -qw "$sym" <<< "$EXPORTS"; then
    echo "ERROR: Required export missing: $sym" >&2
    exit 1
  fi
done
echo "All ${#REQUIRED_EXPORTS[@]} required exports present."

echo "=== Section & File Size Summary ==="
"${HOST}-size" -A "$OUT_DIR/libmpv-2.dll"
ls -lh "$OUT_DIR/libmpv-2.dll"

7z a -t7z -mx=9 "$OUT_DIR/libmpv-2-tier1-x64.7z" "$OUT_DIR/libmpv-2.dll" >/dev/null
(
  cd "$OUT_DIR"
  sha256sum libmpv-2.dll libmpv-2-tier1-x64.7z > SHA256SUMS.txt
)
cat "$OUT_DIR/SHA256SUMS.txt"
ls -lh "$OUT_DIR"
