// OmniSuck helper: Chrome (Native Messaging) -> yt-dlp / direct file downloads.
package main

import (
	"bufio"
	"runtime"
	"strconv"
	"sync"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Item struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type Msg struct {
	Action  string `json:"action"`
	URL     string `json:"url"`
	Quality string `json:"quality"`
	Format  string `json:"format"`
	Folder  string `json:"folder"`
	Items   []Item `json:"items"`
}

var home, _ = os.UserHomeDir()
var app = appDir()
var bin = filepath.Join(app, "bin")

func reply(v map[string]any) {
	b, _ := json.Marshal(v)
	binary.Write(os.Stdout, binary.LittleEndian, uint32(len(b)))
	os.Stdout.Write(b)
}

func fail(e string) { reply(map[string]any{"ok": false, "error": e}) }

func folder(f string) string {
	if f == "" {
		f = "~/Downloads"
	}
	if strings.HasPrefix(f, "~") {
		f = filepath.Join(home, f[1:])
	}
	os.MkdirAll(f, 0755)
	return f
}

// update yt-dlp at most every 3 days (sites change often)
func maybeUpdate() {
	stamp := filepath.Join(app, ".updated")
	if st, err := os.Stat(stamp); err == nil && time.Since(st.ModTime()) < 72*time.Hour {
		return
	}
	c := exec.Command(filepath.Join(bin, "yt-dlp"+exe), "-U")
	c.Start()
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		c.Process.Kill()
	}
	os.WriteFile(stamp, []byte(time.Now().String()), 0644)
}

var outMu sync.Mutex

func progress(stage string, pct float64) {
	outMu.Lock()
	defer outMu.Unlock()
	reply(map[string]any{"progress": int(pct), "stage": stage})
}

// reads a stream line by line (\n or \r) and calls fn for each line
func lines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Split(func(d []byte, eof bool) (int, []byte, error) {
		for i, c := range d {
			if c == '\n' || c == '\r' {
				return i + 1, d[:i], nil
			}
		}
		if eof && len(d) > 0 {
			return len(d), d, nil
		}
		return 0, nil, nil
	})
	for sc.Scan() {
		fn(sc.Text())
	}
}

func video(m Msg) {
	maybeUpdate()
	dir := folder(m.Folder)
	args := []string{"--no-playlist", "--newline", "--progress", "--progress-template", "download:SFP %(progress._percent_str)s",
		"--ffmpeg-location", bin, "-P", dir, "-o", "%(title).80B [%(id)s].%(ext)s",
		"--print", "after_move:SFFILE %(filepath)s", "--encoding", "utf-8"}
	if m.Format == "audio" {
		args = append(args, "-x", "--audio-format", "mp3")
	} else {
		sort := "res,vcodec:h264,acodec:m4a"
		if m.Quality != "" && m.Quality != "best" {
			sort = "res:" + m.Quality + ",vcodec:h264,acodec:m4a"
		}
		args = append(args, "-f", "bv*+ba/b", "-S", sort, "--merge-output-format", "mp4")
	}
	args = append(args, m.URL)
	c := exec.Command(filepath.Join(bin, "yt-dlp"+exe), args...)
	hide(c)
	so, _ := c.StdoutPipe()
	se, _ := c.StderrPipe()
	var file, errText string
	var mu sync.Mutex
	parse := func(l string) {
		l = strings.TrimSpace(l)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasPrefix(l, "SFP "):
			if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(l[4:]), "%"), 64); err == nil {
				progress("download", v)
			}
		case strings.HasPrefix(l, "SFFILE "):
			file = strings.TrimSpace(l[7:])
		case strings.Contains(l, "ERROR:"):
			errText = l[strings.Index(l, "ERROR:"):]
		}
	}
	if err := c.Start(); err != nil {
		fail("Не запускается yt-dlp. Запустите установщик ещё раз.")
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { lines(so, parse); wg.Done() }()
	go func() { lines(se, parse); wg.Done() }()
	wg.Wait()
	if err := c.Wait(); err != nil || file == "" {
		if len(errText) > 300 {
			errText = errText[:300]
		}
		fail("Не получилось скачать. " + errText)
		return
	}
	if m.Format != "audio" {
		file = toH264(file)
	}
	reply(map[string]any{"ok": true, "text": "Сохранено: " + filepath.Base(file)})
}

func files(m Msg) {
	dir := folder(m.Folder)
	n := 0
	for _, it := range m.Items {
		req, _ := http.NewRequest("GET", it.URL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh)")
		r, err := http.DefaultClient.Do(req)
		if err != nil || r.StatusCode != 200 {
			continue
		}
		f, err := os.Create(filepath.Join(dir, filepath.Base(it.Name)))
		if err == nil {
			io.Copy(f, r.Body)
			f.Close()
			n++
		}
		r.Body.Close()
	}
	if n == 0 {
		fail("Не получилось скачать файлы из поста.")
		return
	}
	reply(map[string]any{"ok": true, "text": fmt.Sprintf("Сохранено файлов: %d (папка %s)", n, filepath.Base(dir))})
}

func main() {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[1], "chrome-extension://") {
		install() // started by double-click (Windows installer)
		return
	}
	var n uint32
	if binary.Read(os.Stdin, binary.LittleEndian, &n) != nil {
		return
	}
	buf := make([]byte, n)
	io.ReadFull(os.Stdin, buf)
	var m Msg
	if json.Unmarshal(buf, &m) != nil {
		fail("bad message")
		return
	}
	switch m.Action {
	case "ping":
		reply(map[string]any{"ok": true})
	case "pick":
		f, err := pickFolder()
		if err != nil || f == "" {
			fail("cancel")
			return
		}
		reply(map[string]any{"ok": true, "folder": f})
	case "files":
		files(m)
	default:
		video(m)
	}
}

// YouTube max quality is often VP9/AV1, which QuickTime and editors can't open.
// Re-encode such files to H.264 — with the hardware encoder when possible (low CPU).
func toH264(file string) string {
	probe := func(entry string) string {
		pr := exec.Command(filepath.Join(bin, "ffprobe"+exe), "-v", "error", "-select_streams", "v:0",
			"-show_entries", entry, "-of", "csv=p=0", file)
		hide(pr)
		o, _ := pr.Output()
		return strings.TrimSpace(strings.Split(string(o), "\n")[0])
	}
	if c := probe("stream=codec_name"); c == "" || c == "h264" {
		return file
	}
	dur, _ := strconv.ParseFloat(probe("format=duration"), 64)
	base := strings.TrimSuffix(file, filepath.Ext(file))
	tmp := base + ".h264.tmp.mp4"
	ff := filepath.Join(bin, "ffmpeg"+exe)
	// use at most ~80% of the processor: limit decoder + encoder threads
	lim := runtime.NumCPU() * 8 / 10
	if lim < 1 {
		lim = 1
	}
	for _, enc := range encoders(lim) {
		args := []string{"-y", "-v", "error", "-nostats", "-progress", "pipe:1", "-threads", strconv.Itoa(lim)}
		args = append(append(append(args, hwDecode...), "-i", file), enc...)
		args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", tmp)
		c := exec.Command(ff, args...)
		hide(c)
		lowPriority(c)
		so, _ := c.StdoutPipe()
		if c.Start() != nil {
			continue
		}
		lines(so, func(l string) {
			if strings.HasPrefix(l, "out_time_us=") && dur > 0 {
				if us, err := strconv.ParseFloat(l[12:], 64); err == nil {
					progress("convert", us/1e6/dur*100)
				}
			}
		})
		if c.Wait() == nil {
			os.Remove(file)
			os.Rename(tmp, base+".mp4")
			return base + ".mp4"
		}
		os.Remove(tmp)
	}
	return file
}
