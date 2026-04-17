package main

import (
	"fmt"
	"net/http"

	"github.com/jqtmviyu/iinaServer/internal/config"
	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/review"
	"github.com/jqtmviyu/iinaServer/internal/session"
)

func main() {
	cfg := config.ParseFlags()
	store := review.NewStore(20)
	sessions := session.NewManager()

	mux := http.NewServeMux()
	handler := httpapi.Handler{
		Store:      store,
		DirectPlay: newDirectPlayHandler(cfg, store, sessions),
		EmbyPlay:   newEmbyPlayHandler(cfg, store, sessions),
		Stop:       newStopHandler(store, sessions),
	}
	handler.Register(mux)

	fmt.Printf("服务已启动，监听在 :%s 端口...\n", cfg.Port)
	fmt.Printf("需要修改端口使用 -port=xxx 参数\n")
	fmt.Println("通过 http://localhost:" + cfg.Port + "/play?video=xxx&subtitle=xxx 播放视频")
	fmt.Println("健康检查: http://localhost:" + cfg.Port + "/healthz")
	fmt.Println("最近一次 review: http://localhost:" + cfg.Port + "/v1/review/sessions/latest")
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		panic(err)
	}
}
