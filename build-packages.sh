#!/usr/bin/env bash
set -euo pipefail

VERSION="1.0.3"
ARCH="amd64"
RPM_ARCH="x86_64"
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUT_DIR="${ROOT_DIR}/dist"

echo "==> Building codex-unpacker v${VERSION}..."
go build -trimpath -o "${OUT_DIR}/codex-unpacker" .

# ── .deb ────────────────────────────────────────────────────────────
echo "==> Packaging .deb..."
DEB_ROOT="${OUT_DIR}/deb-staging"
rm -rf "${DEB_ROOT}"
cp -a "${ROOT_DIR}/packaging/deb" "${DEB_ROOT}"
install -m 0755 "${OUT_DIR}/codex-unpacker" "${DEB_ROOT}/usr/bin/codex-unpacker"
install -m 0644 "${ROOT_DIR}/assets/logo.png" \
    "${DEB_ROOT}/usr/share/icons/hicolor/512x512/apps/codex-unpacker.png"

fakeroot dpkg-deb --build --root-owner-group "${DEB_ROOT}" "${OUT_DIR}/codex-unpacker_${VERSION}_${ARCH}.deb"
echo "    -> $(ls -lh "${OUT_DIR}/codex-unpacker_${VERSION}_${ARCH}.deb")"

# ── .rpm ────────────────────────────────────────────────────────────
echo "==> Packaging .rpm..."
RPM_TOPDIR="${OUT_DIR}/rpm-build"
rm -rf "${RPM_TOPDIR}"
mkdir -p "${RPM_TOPDIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS,BUILDROOT}

install -m 0755 "${OUT_DIR}/codex-unpacker" "${RPM_TOPDIR}/SOURCES/codex-unpacker"
install -m 0644 "${ROOT_DIR}/packaging/deb/usr/share/applications/codex-unpacker.desktop" \
    "${RPM_TOPDIR}/SOURCES/codex-unpacker.desktop"
install -m 0644 "${ROOT_DIR}/assets/logo.png" \
    "${RPM_TOPDIR}/SOURCES/codex-unpacker.png"

rpmbuild --define "_topdir ${RPM_TOPDIR}" \
         --define "_bindir /usr/bin" \
         --define "_datadir /usr/share" \
         -bb "${ROOT_DIR}/packaging/rpm/codex-unpacker.spec"

cp "${RPM_TOPDIR}/RPMS/${RPM_ARCH}/codex-unpacker-${VERSION}-1.${RPM_ARCH}.rpm" \
   "${OUT_DIR}/codex-unpacker-${VERSION}-1.${RPM_ARCH}.rpm"
echo "    -> $(ls -lh "${OUT_DIR}/codex-unpacker-${VERSION}-1.${RPM_ARCH}.rpm")"

# ── checksums ───────────────────────────────────────────────────────
echo "==> Checksums:"
cd "${OUT_DIR}"
sha256sum codex-unpacker_${VERSION}_${ARCH}.deb codex-unpacker-${VERSION}-1.${RPM_ARCH}.rpm \
    > SHA256SUMS.txt
cat SHA256SUMS.txt

echo ""
echo "==> Done. Packages in ${OUT_DIR}/"
