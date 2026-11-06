package main

import (
	"fmt"
	"net"
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

	fmt.Printf("服务已启动，监听在 127.0.0.1:%s 端口...\n", cfg.Port)
	fmt.Printf("需要修改端口使用 -port=xxx 参数\n")
	fmt.Println("通过 http://127.0.0.1:" + cfg.Port + "/play?video=xxx&subtitle=xxx 播放视频")
	fmt.Println("健康检查: http://127.0.0.1:" + cfg.Port + "/healthz")
	fmt.Println("最近一次 review: http://127.0.0.1:" + cfg.Port + "/v1/review/sessions/latest")
	addr := net.JoinHostPort("127.0.0.1", cfg.Port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
