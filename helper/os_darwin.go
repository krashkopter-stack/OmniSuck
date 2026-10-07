package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const exe = ""

func appDir() string { return filepath.Join(home, "Library", "Application Support", "OmniSuck") }
func hide(c *exec.Cmd) {}
func install()         { fmt.Println("Используйте «Установить.command».") }

func pickFolder() (string, error) {
	out, err := exec.Command("osascript", "-e",
		`tell application (path to frontmost application as text) to POSIX path of (choose folder with prompt "Куда сохранять видео?")`).Output()
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), err
}

func encoders(threads int) [][]string {
	return [][]string{
		{"-c:v", "h264_videotoolbox", "-q:v", "65"},
		{"-c:v", "h264_videotoolbox", "-b:v", "20M"},
		{"-c:v", "libx264", "-preset", "veryfast", "-crf", "19", "-threads", fmt.Sprint(threads)},
	}
}

func lowPriority(c *exec.Cmd) {
	c.Path, c.Args = "/usr/bin/nice", append([]string{"nice", "-n", "15", c.Path}, c.Args[1:]...)
}

// hardware video decoding (falls back to the processor automatically)
var hwDecode = []string{"-hwaccel", "videotoolbox"}
