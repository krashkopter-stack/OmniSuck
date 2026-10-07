#!/bin/bash
# Установщик OmniSuck: помощник + yt-dlp + ffmpeg + расширение
cd "$(dirname "$0")"
APP="$HOME/Library/Application Support/OmniSuck"; BIN="$APP/bin"
EXT="$HOME/Documents/OmniSuck Extension"
ID="peeinmpdfamgbkeiadheneobcaoiecmd"
fail(){ echo; echo "❌ $1"; echo "Проверьте интернет и запустите установку ещё раз."; read -n1 -p "Нажмите любую клавишу…"; exit 1; }
echo "=== Установка OmniSuck ==="
mkdir -p "$BIN" || fail "Не удалось создать папку"
if [ "$(uname -m)" = arm64 ]; then A=arm64; else A=amd64; fi
cp -f "files/helper-$A" "$APP/omnisuck-helper" && chmod +x "$APP/omnisuck-helper" || fail "Не удалось скопировать помощника"

echo "1/3 Устанавливаю загрузчик видео (yt-dlp)…"
rm -rf "$BIN/yt-dlp" "$BIN/yt-dlp-app"
PY=""
for c in /opt/homebrew/bin/python3 /usr/local/bin/python3 /Library/Frameworks/Python.framework/Versions/Current/bin/python3 /usr/bin/python3; do
  [ -x "$c" ] || continue
  [ "$c" = /usr/bin/python3 ] && ! xcode-select -p >/dev/null 2>&1 && continue
  "$c" -c 'import sys; sys.exit(sys.version_info < (3,9))' 2>/dev/null && { PY="$c"; break; }
done
if [ -n "$PY" ]; then
  "$PY" -m venv "$APP/venv" && "$APP/venv/bin/python" -m pip install -q -U pip yt-dlp || fail "Не установился yt-dlp"
  cat > "$BIN/yt-dlp" <<'W'
#!/bin/bash
V="$HOME/Library/Application Support/OmniSuck/venv/bin/python"
if [ "$1" = "-U" ]; then exec "$V" -m pip install -q -U yt-dlp; fi
exec "$V" -m yt_dlp "$@"
W
else
  # нет Python — берём готовую распакованную версию yt-dlp
  curl -fL --progress-bar -o /tmp/sf_ytdlp.zip "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos.zip" || fail "Не скачался yt-dlp"
  mkdir -p "$BIN/yt-dlp-app" && unzip -o -q /tmp/sf_ytdlp.zip -d "$BIN/yt-dlp-app" && rm -f /tmp/sf_ytdlp.zip
  xattr -cr "$BIN/yt-dlp-app" 2>/dev/null
  E=$(find "$BIN/yt-dlp-app" -name 'yt-dlp_macos' -type f | head -1)
  [ -n "$E" ] || fail "Не найден yt-dlp в архиве"
  chmod +x "$E"; printf '#!/bin/bash\nexec "%s" "$@"\n' "$E" > "$BIN/yt-dlp"
fi
chmod +x "$BIN/yt-dlp"

echo "2/3 Скачиваю ffmpeg (склейка видео и звука)…"
for t in ffmpeg ffprobe; do
  if [ ! -x "$BIN/$t" ]; then
    curl -fL --progress-bar -o "/tmp/sf_$t.zip" "https://ffmpeg.martin-riedl.de/redirect/latest/macos/$A/release/$t.zip" || fail "Не скачался $t"
    unzip -o -q "/tmp/sf_$t.zip" -d "$BIN" && rm -f "/tmp/sf_$t.zip"
    chmod +x "$BIN/$t"
  fi
done
xattr -dr com.apple.quarantine "$APP" 2>/dev/null

echo "3/3 Подключаю к браузерам…"
for B in "Google/Chrome" "Chromium" "Yandex/YandexBrowser" "BraveSoftware/Brave-Browser" "Microsoft Edge"; do
  D="$HOME/Library/Application Support/$B"
  [ -d "$D" ] || continue
  mkdir -p "$D/NativeMessagingHosts"
  cat > "$D/NativeMessagingHosts/com.omnisuck.helper.json" <<J
{"name":"com.omnisuck.helper","description":"OmniSuck","path":"$APP/omnisuck-helper","type":"stdio","allowed_origins":["chrome-extension://$ID/"]}
J
done
rm -rf "$EXT" && cp -R files/extension "$EXT"

"$BIN/yt-dlp" --version >/dev/null 2>&1 || fail "yt-dlp не запускается"
echo
echo "✅ Готово! Осталось добавить расширение в Chrome (см. Инструкция.txt):"
echo "   chrome://extensions → «Режим разработчика» → «Загрузить распакованное»"
echo "   → выбрать папку «Документы / OmniSuck Extension»"
open "$HOME/Documents"
read -n1 -p "Нажмите любую клавишу, чтобы закрыть…"
