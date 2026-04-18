# IINAServer

## 功能

- 点 Emby Web 里的播放按钮，自动交给 IINA
- 尽量复用已有 IINA 窗口，不重复开新窗口
- 支持播放进度回传 Emby

## 流程

点击emby网页版上的播放按钮 -> 暴力猴脚本拦截事件, 构造资源url发送给本地服务器 -> 服务器拉起iina, 播放视频 -> 每隔5秒获取当前播放进度, 每隔30秒上传一次进度

## 快速开始

### 1. 启动服务

```bash
# 二进制运行
./IINAServer_darwin_amd64

# 源码调试
go run .
```

默认端口：`8080`, 启动后可访问：

```text
http://127.0.0.1:8080/healthz
```

看到 `{"status":"ok"}` 说明服务正常。

#### 启动参数

- `-port`: 服务端口
- `-iina-bin`: iina-cli 可执行文件路径
- `-poll-interval`: ipc 读取间隔
- `-progress-interval`: 多少秒上传一次进度

#### IINA 查找顺序

如果你没有传 `-iina-bin`，服务会按下面顺序找：

1. `~/.local/bin/iina-cli`
2. `/Applications/IINA.app/Contents/MacOS/iina-cli`
3. `iina-cli`
4. `iina`

### 2. 安装 userscript

项目自带脚本：

```text
scripts/emby-iina-only.user.js
```

把脚本安装到浏览器 userscript 插件里。

### 3. 改脚本里的本地服务地址

脚本默认配置：

```js
const config = {
  localServer: "http://127.0.0.1:8080",
  reviewOnly: false,
  dryRunUpload: false, // `false`：上传播放进度到 Emby  `true`：只本地记录，不上传
  debugMediaURL: "",
  debugSubtitleURL: "",
  dedupeWindowMs: 2500,
  requestTimeoutMs: 4000,
  debug: false,
};
```

### 4. 在 Emby Web 里点播放

脚本会接管播放按钮，把请求转给本地 IINAServer，再由 IINA 播放。

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
  - 可看到该次播放的参数、采样、进度上传、错误信息

- `POST /v1/session/stop`
  - 停止当前本地接管会话
  - 一般由 userscript 自动调用

