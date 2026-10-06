#!/bin/sh
# Packs the Linux application two ways: a .deb for Debian, Ubuntu and their relatives, and an
# archive with a small installer for everything else.
#
#   packaging/linux.sh <the built program> <version> <folder to write to>
set -eu
program=$1
version=$2
out=$3
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

go run ./tools/icon "$work/c-bass.png"

desktop() { # $1: what starts the program
	cat <<END
[Desktop Entry]
Type=Application
Name=C_bass
GenericName=Bass transcriber
GenericName[it]=Trascrittore per basso
Comment=From a recording to a bass part: tablature, neck, play-along
Comment[it]=Dal brano alla parte di basso: tablatura, manico, base per suonarci sopra
Exec=$1
Icon=c-bass
Terminal=false
Categories=AudioVideo;Audio;Music;Education;
Keywords=bass;tab;tablature;transcribe;basso;tablatura;
END
}

# --- the .deb ---
# A Debian version starts with a digit and keeps "-" for the packaging revision.
debversion=$(printf %s "$version" | sed 's/^v//; s/-/~/g')
case $debversion in [0-9]*) ;; *) debversion="0.0.0~$debversion" ;; esac
deb="$work/deb"
install -D -m 755 "$program" "$deb/usr/bin/c-bass"
install -D -m 644 "$work/c-bass.png" "$deb/usr/share/icons/hicolor/512x512/apps/c-bass.png"
mkdir -p "$deb/usr/share/applications" "$deb/usr/share/doc/c-bass" "$deb/DEBIAN"
desktop c-bass > "$deb/usr/share/applications/c-bass.desktop"
cat > "$deb/usr/share/doc/c-bass/copyright" <<END
C_bass, by Massimo Danieli: https://github.com/MassimoDanieli/C_bass
The program is free software under the GNU General Public License, version 3 or later:
see /usr/share/common-licenses/GPL-3.
On first use it downloads ONNX Runtime (MIT) and a Demucs model whose weights, from Meta,
are for personal and research use.
END
cat > "$deb/DEBIAN/control" <<END
Package: c-bass
Version: $debversion
Architecture: amd64
Maintainer: Massimo Danieli <massimo.danieli@gmail.com>
Section: sound
Priority: optional
Homepage: https://github.com/MassimoDanieli/C_bass
Installed-Size: $(du -sk "$deb/usr" | cut -f1)
Depends: libc6 (>= 2.34), libx11-6, libxrandr2, libxcursor1, libxinerama1, libxi6, libxxf86vm1, libgl1, libasound2t64 | libasound2
Recommends: zenity | kdialog
Description: From a recording to a bass part
 C_bass separates the bass from a recording, reads its notes and writes them
 as a tablature that scrolls while the recording plays, with the neck showing
 the note to play. The speed can be lowered without changing the pitch, a
 stretch of bars repeated, and the bass turned down to play along.
END
dpkg-deb --root-owner-group --build "$deb" "$out/C_bass-linux-x64.deb" > /dev/null

# --- the archive ---
tar="$work/C_bass-linux-x64"
mkdir -p "$tar"
install -m 755 "$program" "$tar/c-bass"
cp "$work/c-bass.png" "$tar/c-bass.png"
cat > "$tar/install.sh" <<'END'
#!/bin/sh
# Installs C_bass for this user only: nothing is written outside the home folder.
#   ./install.sh             installs
#   ./install.sh --remove    takes it away again
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bin="${XDG_BIN_HOME:-$HOME/.local/bin}"
data="${XDG_DATA_HOME:-$HOME/.local/share}"
if [ "${1:-}" = "--remove" ]; then
	rm -f "$bin/c-bass" "$data/applications/c-bass.desktop" "$data/icons/hicolor/512x512/apps/c-bass.png"
	echo "C_bass removed. The recordings it analysed are still in ${XDG_CONFIG_HOME:-$HOME/.config}/C_bass."
	exit 0
fi
mkdir -p "$bin" "$data/applications" "$data/icons/hicolor/512x512/apps"
install -m 755 "$here/c-bass" "$bin/c-bass"
install -m 644 "$here/c-bass.png" "$data/icons/hicolor/512x512/apps/c-bass.png"
sed "s|^Exec=.*|Exec=$bin/c-bass|" "$here/c-bass.desktop" > "$data/applications/c-bass.desktop"
command -v update-desktop-database > /dev/null 2>&1 && update-desktop-database "$data/applications" > /dev/null 2>&1 || true
echo "C_bass installed: look for it among the applications, or run $bin/c-bass"
END
chmod 755 "$tar/install.sh"
desktop c-bass > "$tar/c-bass.desktop"
cat > "$tar/README.txt" <<END
C_bass $version for Linux (x86-64)

  ./install.sh            installs it for you alone, in ~/.local
  ./install.sh --remove   takes it away again
  ./c-bass                runs it from here, without installing

It needs X11 (or XWayland), OpenGL and ALSA, which a desktop already has, and zenity or
kdialog for the "Choose a file" panel; a recording can always be dropped on the window.

https://github.com/MassimoDanieli/C_bass
END
tar -C "$work" -czf "$out/C_bass-linux-x64.tar.gz" C_bass-linux-x64
ls -l "$out/C_bass-linux-x64.deb" "$out/C_bass-linux-x64.tar.gz"
