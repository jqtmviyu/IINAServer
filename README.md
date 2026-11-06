# IINAServer

## 功能

- 点 Emby Web 里的播放按钮，交给 IINA 或 MPV 播放
- 通过油猴菜单切换当前播放器并记住选择
- 尽量复用同类型播放器进程
- 支持自动向 Emby 回传播放进度

## 流程

点击 Emby Web 的播放按钮 → userscript 根据当前设置选择 IINA 或 MPV → 本地 Go 服务通过播放器 IPC 每 5 秒采样，并按 30 秒间隔向 Emby 上报进度。

## 快速开始

### 1. 启动服务

```bash
# 二进制运行
./IINAServer_darwin_amd64

# 源码调试
go run .
```

默认绑定 `127.0.0.1:8080`，只接受本机连接。油猴从 Emby 页面发请求时，浏览器会自动附带页面 Origin；本地服务按该 Origin 返回 CORS 响应，不需要在 launchd 中配置或手工传递域名。

启动后可访问：

```text
http://127.0.0.1:8080/healthz
```

看到 `{"status":"ok"}` 说明服务正常。

#### 启动参数

- `-port`: 本地服务端口，默认 `8080`
- `-iina-bin`: IINA CLI 可执行文件路径
- `-mpv-bin`: MPV 可执行文件路径；留空时从 `PATH` 查找 `mpv`
- `-ipc-dir`: IPC socket 目录；默认放在当前用户的缓存目录并限制为仅用户可访问
- `-poll-interval`: IPC 状态采样间隔，默认 5 秒
- `-progress-interval`: 向 Emby 上报进度的间隔，默认 30 秒

#### IINA 查找顺序

如果你没有传 `-iina-bin`，服务会按下面顺序找：

1. `~/.local/bin/iina-cli`
2. `/Applications/IINA.app/Contents/MacOS/iina-cli`
3. `iina-cli`
4. `iina`

MPV 默认从 `PATH` 查找 `mpv`，也可用 `-mpv-bin=/path/to/mpv` 指定。

### 2. 安装 userscript

项目自带脚本：

```text
scripts/emby-iina-only.user.js
```

把脚本安装到浏览器 userscript 插件里。

### 3. 切换 IINA / MPV

脚本默认使用 MPV。打开油猴/暴力猴的脚本菜单，选择“切换播放器（IINA / MPV）”即可切换；选择保存在 userscript 管理器里，刷新页面或重启浏览器后仍会保留。Emby 页面保持一个播放入口，之后的新播放会使用当前选中的播放器。

脚本默认连接 `http://127.0.0.1:8080`。如果修改了服务地址，需要同时确保该地址允许本机访问。

### 4. 在 Emby Web 里点播放

脚本读取 Emby 当前登录态获取播放信息，将选中的播放器和播放计划交给本地服务。默认会把播放开始、进度和停止事件真实上报到 Emby；测试时编辑脚本将 `dryRunUpload` 设为 `true`，只记录不上传。该显式值会覆盖服务端 `-upload-mode` 默认值。`reviewOnly` 可只检查播放计划而不启动播放器。

## 构建

```bash
bash scripts/build.sh darwin-arm64
bash scripts/build.sh darwin-amd64
bash scripts/build.sh all
```

产物在 `build/` 目录。

## 更多api

- `GET /healthz`
  - 健康检查
  - 用来确认服务是否启动成功

- `GET /play?video=<视频URL>&subtitle=<字幕URL>`
  - direct play 接口
  - 不走 Emby Web，直接让本地 IINA 播放指定视频
  - `video` 必填，`subtitle` 可选

- `GET /v1/review/sessions/latest`
  - 查看最近一次播放接管记录
  - 适合排查“有没有接管成功”“有没有上传进度”

- `GET /v1/review/sessions/{sessionId}`
  - 查看指定会话详情
  - 可看到播放器类型、采样、进度上传结果和错误信息；鉴权 token 会脱敏

- `POST /v1/emby/play`
  - 根据请求中的 `options.player`（`iina` 或 `mpv`）启动对应播放器
  - 脚本携带 Emby 播放信息，服务负责解析流地址并采样进度

- `POST /v1/session/stop`
  - 停止当前本地接管会话
  - 一般由 userscript 自动调用

