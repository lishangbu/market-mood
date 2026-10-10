# 本机 Web 服务

本机仅用于开发，运行 Web 服务，通过 http://127.0.0.1:18080 访问。最终部署目标为 Linux amd64 服务器。

- 用户服务：`market-mood-web.service`，仅手动启动，未启用登录自启动；运行期间异常退出自动重启。
- 可执行文件：`~/.local/lib/market-mood-web/market-mood`。
- 数据目录：`~/.local/share/market-mood-web`。
- 服务配置：`~/.config/systemd/user/market-mood-web.service`。
- Go 源码：`仓库工作目录`。

没有桌面快捷方式、应用菜单条目或启动浏览器的桌面启动器。
首次启动会采集行情、同步目录并计算股票池。

```bash
systemctl --user start market-mood-web.service
systemctl --user status market-mood-web.service
systemctl --user restart market-mood-web.service
systemctl --user stop market-mood-web.service
journalctl --user -u market-mood-web.service -n 50 --no-pager
```

服务仅监听本机回环地址。评分以页面标示交易日和数据时间为准；休市时显示最近交易日行情。

Linux amd64 发布产物为 `bin/market-mood-linux-amd64`，内嵌前端资源，无需服务器安装 Node.js 或 Go。发布前更新前端资源并构建：

```bash
make frontend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/market-mood-linux-amd64 ./cmd/market-mood
```
