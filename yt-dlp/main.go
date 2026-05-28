package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("===================================")
	fmt.Println("   🎬 Smart Media Downloader")
	fmt.Println("===================================")

	// Ask URL
	fmt.Print("Enter Video URL: ")
	url, _ := reader.ReadString('\n')
	url = strings.TrimSpace(url)

	if url == "" {
		log.Fatal("URL cannot be empty")
	}

	// Ask output path
	fmt.Print("Enter Save Directory (example: /home/user/Videos): ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)

	if path == "" {
		log.Fatal("Path cannot be empty")
	}

	fmt.Println("\n🚀 Downloading BEST quality...")
	fmt.Println("Video: Best Available")
	fmt.Println("Audio: Best Available")
	fmt.Println("Merging with FFmpeg...\n")

	// yt-dlp command
	cmd := exec.Command(
		"yt-dlp",
		"-f", "bv*+ba/b",
		"--merge-output-format", "mkv",
		"-o", path+"/%(title)s.%(ext)s",
		url,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		log.Fatal("❌ Download failed:", err)
	}

	fmt.Println("\n✅ Done. Highest quality video saved!")
}
