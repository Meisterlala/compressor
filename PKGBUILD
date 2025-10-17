# Maintainer: Misti <your@email>
pkgname=compressor-git
pkgver=r7.ba24d78
pkgrel=1
pkgdesc="Video compressor service that watches input directory and compresses videos using ffmpeg"
arch=('x86_64' 'aarch64')
license=('MIT')
depends=('ffmpeg')
makedepends=('go')

# 👇 this captures your repo path before makepkg changes directories
_repopath="$(pwd)"

pkgver() {
  cd "$_repopath"
  printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

build() {
  cd "$_repopath"
  go build -o "$srcdir/compressor" ./cmd/compressor
}

package() {
  cd "$_repopath"
  install -Dm755 "$srcdir/compressor" "$pkgdir/usr/bin/compressor"
  install -Dm644 compressor.service "$pkgdir/usr/lib/systemd/user/compressor.service"
}
