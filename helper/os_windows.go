package main

import (
	"archive/zip"
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const exe = ".exe"

//go:embed ext
var extFS embed.FS

func appDir() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "OmniSuck") }

func hide(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func lowPriority(c *exec.Cmd) { c.SysProcAttr.CreationFlags |= 0x00004000 } // BELOW_NORMAL_PRIORITY_CLASS

func encoders(threads int) [][]string {
	return [][]string{
		{"-c:v", "h264_nvenc", "-preset", "p5", "-cq", "21"},
		{"-c:v", "h264_qsv", "-global_quality", "22"},
		{"-c:v", "h264_amf", "-rc", "cqp", "-qp_i", "20", "-qp_p", "22"},
		{"-c:v", "libx264", "-preset", "veryfast", "-crf", "19", "-threads", fmt.Sprint(threads)},
	}
}

func pickFolder() (string, error) {
	ps := `[Console]::OutputEncoding=[Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms;` +
		`$o=New-Object System.Windows.Forms.Form -Property @{TopMost=$true;ShowInTaskbar=$false};` +
		`$d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Куда сохранять видео?';` +
		`if($d.ShowDialog($o) -eq 'OK'){ $d.SelectedPath }`
	c := exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
	hide(c)
	out, err := c.Output()
	return strings.TrimSpace(string(out)), err
}

func wait() { fmt.Print("\nНажмите Enter, чтобы закрыть…"); bufio.NewReader(os.Stdin).ReadString('\n') }

func die(msg string) {
	fmt.Println("\n❌ " + msg + "\nПроверьте интернет и запустите установку ещё раз.")
	wait()
	os.Exit(1)
}

func download(url, to string) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", r.StatusCode)
	}
	f, err := os.Create(to)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 256*1024)
	var got int64
	for {
		n, e := r.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
			got += int64(n)
			if r.ContentLength > 0 {
				fmt.Printf("\r   %d%%  ", got*100/r.ContentLength)
			} else {
				fmt.Printf("\r   %d МБ  ", got>>20)
			}
		}
		if e == io.EOF {
			fmt.Println()
			return nil
		}
		if e != nil {
			return e
		}
	}
}

func unzipBins(zipPath string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer z.Close()
	n := 0
	for _, f := range z.File {
		b := filepath.Base(f.Name)
		if b != "ffmpeg.exe" && b != "ffprobe.exe" {
			continue
		}
		rc, _ := f.Open()
		out, err := os.Create(filepath.Join(bin, b))
		if err == nil {
			io.Copy(out, rc)
			out.Close()
			n++
		}
		rc.Close()
	}
	if n < 2 {
		return fmt.Errorf("в архиве нет ffmpeg")
	}
	return nil
}

func install() {
	fmt.Println("=== Установка OmniSuck ===")
	if err := os.MkdirAll(bin, 0755); err != nil {
		die("Не удалось создать папку")
	}
	helper := filepath.Join(app, "omnisuck-helper.exe")
	self, _ := os.Executable()
	if !strings.EqualFold(self, helper) {
		data, _ := os.ReadFile(self)
		if os.WriteFile(helper, data, 0755) != nil {
			die("Не удалось скопировать помощника (закройте Chrome и попробуйте снова)")
		}
	}

	fmt.Println("1/3 Скачиваю загрузчик видео (yt-dlp)…")
	if err := download("https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe", filepath.Join(bin, "yt-dlp.exe")); err != nil {
		die("Не скачался yt-dlp: " + err.Error())
	}

	fmt.Println("2/3 Скачиваю ffmpeg (склейка видео и звука)…")
	if _, err := os.Stat(filepath.Join(bin, "ffmpeg.exe")); err != nil {
		tmp := filepath.Join(os.TempDir(), "sf_ffmpeg.zip")
		err := download("https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip", tmp)
		if err != nil {
			err = download("https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip", tmp)
		}
		if err == nil {
			err = unzipBins(tmp)
		}
		os.Remove(tmp)
		if err != nil {
			die("Не скачался ffmpeg: " + err.Error())
		}
	} else {
		fmt.Println("   уже есть")
	}

	fmt.Println("3/3 Подключаю к браузерам…")
	man := filepath.Join(app, "com.omnisuck.helper.json")
	j, _ := json.Marshal(map[string]any{"name": "com.omnisuck.helper", "description": "OmniSuck", "path": helper,
		"type": "stdio", "allowed_origins": []string{"chrome-extension://" + extID + "/"}})
	os.WriteFile(man, j, 0644)
	for _, k := range []string{`Google\Chrome`, `Chromium`, `Microsoft\Edge`, `Yandex\YandexBrowser`, `BraveSoftware\Brave-Browser`} {
		c := exec.Command("reg", "add", `HKCU\Software\`+k+`\NativeMessagingHosts\com.omnisuck.helper`, "/ve", "/t", "REG_SZ", "/d", man, "/f")
		hide(c)
		c.Run()
	}

	ext := filepath.Join(home, "Documents", "OmniSuck Extension")
	os.RemoveAll(ext)
	fs.WalkDir(extFS, "ext", func(p string, d fs.DirEntry, _ error) error {
		dst := filepath.Join(ext, strings.TrimPrefix(p, "ext"))
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		b, _ := extFS.ReadFile(p)
		return os.WriteFile(dst, b, 0644)
	})

	fmt.Println("\n✅ Готово! Осталось добавить расширение в Chrome:")
	fmt.Println("   chrome://extensions → «Режим разработчика» → «Загрузить распакованное»")
	fmt.Println("   → выбрать папку «Документы\\OmniSuck Extension» (сейчас откроется)")
	exec.Command("explorer", ext).Start()
	wait()
}

// hardware video decoding (falls back to the processor automatically)
var hwDecode = []string{"-hwaccel", "auto"}
