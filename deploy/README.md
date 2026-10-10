# Linux systemd 部署

先运行 `make release`，将 `bin/market-mood-linux-amd64` 部署到 Linux amd64 服务器。产物内嵌网页，运行无需 Go、Node.js 或 CGo 依赖。

以下命令在目标主机执行（需要管理员权限）：

```sh
sudo install -d /opt/market-mood
sudo install -m 0755 bin/market-mood-linux-amd64 /opt/market-mood/market-mood
sudo install -m 0644 deploy/market-mood.service /etc/systemd/system/market-mood.service
sudo systemctl daemon-reload
sudo systemctl enable --now market-mood
curl -fsS http://127.0.0.1:8080/api/health
journalctl -u market-mood -f
```

unit默认绑定回环地址，需远程访问时可通过反向代理或systemd覆盖ExecStart配置明确监听地址。数据库由StateDirectory管理，位于 `/var/lib/market-mood/market-mood.db`。

升级时先停止服务、替换二进制再启动，保留数据目录。停止命令为 `sudo systemctl stop market-mood`。服务收到SIGTERM后共用10秒排空HTTP及采集，数据库关闭另有1秒；systemd在15秒后清理整个进程组。

`/api/health`检查任务存活，`/api/tasks`还显示单轮来源错误；HTTP200不证明行情新鲜或完整，需同时查看看板来源时间和错误。完整交易日验收尚在进行。
