#!/bin/bash
# Builds dist/OmniSuck.dmg (Mac) and dist/OmniSuck-Windows.zip
set -e
cd "$(dirname "$0")"
rm -rf build dist helper/ext && mkdir -p build/dmg/files dist
cp -r extension helper/ext                     # embedded into the Windows installer
cp -r extension build/dmg/files/extension
cd helper
export CGO_ENABLED=0
GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o ../build/dmg/files/helper-arm64 .
GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o ../build/dmg/files/helper-amd64 .
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ../build/OmniSuck-Setup.exe .
cd ..
cp installer/mac/Установить.command build/dmg/ && chmod +x build/dmg/Установить.command
cp docs/Инструкция.txt build/dmg/
python3 tools/mkiso.py build/dmg dist/OmniSuck.dmg "OmniSuck"
python3 - <<'P'
import zipfile
z = zipfile.ZipFile('dist/OmniSuck-Windows.zip', 'w', zipfile.ZIP_DEFLATED)
z.write('build/OmniSuck-Setup.exe', 'OmniSuck-Setup.exe')
z.write('docs/Инструкция.txt', 'Инструкция.txt')
z.close()
P
rm -rf helper/ext
echo "Готово: dist/"
