# Maintainer: Misti <your@email>
pkgname=compressor-git
pkgver=r4.c3cc640
pkgrel=1
pkgdesc="Video compressor service that watches input directory and compresses videos using ffmpeg"
arch=('x86_64' 'aarch64')
url="https://github.com/meisterlala/compressor"
license=('MIT')
depends=('ffmpeg')
makedepends=('go')

pkgver() {
  printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

build() {
  go build -o "$srcdir/compressor" ./cmd/compressor
}

package() {
  install -Dm755 "$srcdir/compressor" "$pkgdir/usr/bin/compressor"
  install -Dm644 compressor.service "$pkgdir/usr/lib/systemd/user/compressor.service"
}
