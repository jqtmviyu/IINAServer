package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	// 定义端口参数
	port := flag.String("port", "8080", "设置服务监听的端口")
	flag.Parse()

	http.HandleFunc("/play", playHandler)
	fmt.Printf("服务已启动，监听在 :%s 端口...\n", *port)
	fmt.Printf("需要修改端口使用 -port=xxx 参数\n")
	fmt.Println("通过 http://localhost:" + *port + "/play?video=xxx&subtitle=xxx 播放视频")
	http.ListenAndServe(":"+*port, nil)
}

func playHandler(w http.ResponseWriter, r *http.Request) {
	videoURL := r.URL.Query().Get("video")
	fmt.Println("videoURL:", videoURL)
	subtitleURL := r.URL.Query().Get("subtitle")
	fmt.Println("subtitleURL", subtitleURL)

	if videoURL == "" {
		http.Error(w, "缺少视频 URL", http.StatusBadRequest)
		return
	}

	var subtitleFile string
	if subtitleURL != "" {
		// 下载字幕并保存为临时文件
		resp, err := http.Get(subtitleURL)
		if err != nil {
			http.Error(w, "无法下载字幕", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		subtitleData, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "无法读取字幕数据", http.StatusInternalServerError)
			return
		}

		// 提取视频文件名
		subtitleFile = filepath.Join(os.TempDir(), "tmp_sub.ass")

		if err := os.WriteFile(subtitleFile, subtitleData, 0644); err != nil {
			http.Error(w, "无法保存字幕文件", http.StatusInternalServerError)
			return
		}
	}

	// 检查 IINA 是否正在运行
	checkCmd := exec.Command("pgrep", "IINA")
	if err := checkCmd.Run(); err == nil {
		// 如果 IINA 正在运行，则杀死所有相关进程
		killCmd := exec.Command("pkill", "IINA")
		if err := killCmd.Run(); err != nil {
			fmt.Println("杀死 IINA 应用程序失败:", err)
		} else {
			time.Sleep(1 * time.Second) // 等待 1 秒
		}
	}

	cmd := exec.Command("iina", "--no-stdin", videoURL)
	if subtitleFile != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--mpv-sub-files=%s", subtitleFile))
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, "无法启动播放器", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("正在播放视频..."))
}
