# 本地开发

开发环境按需手动启动 Web 服务，不需要安装桌面程序或登录自启动服务。目标部署平台为 Linux amd64。

## 构建和启动

在仓库根目录执行：

```sh
make build
./bin/market-mood --bind 127.0.0.1:8080 --data-dir ./data
```

浏览器访问 http://127.0.0.1:8080 。首次启动会同步目录、采集行情并计算股票池。停止时使用 Ctrl+C。

数据保存在 `./data`，该目录不纳入版本控制。需要隔离测试数据时，通过 `--data-dir` 指定其他目录；升级二进制时保留已有数据目录。

## 前端开发

保持后端运行，在另一个终端执行：

```sh
cd frontend
npm run dev
```

Vite 将 `/api` 代理到后端的 `127.0.0.1:8080`。独立调试前端时使用 Vite 给出的地址；后端内嵌网页在重新构建后更新。

## 增量构建

`make build`、`make test`、`make vet` 和 `make release` 共用前端构建结果：

- `package.json` 或锁文件变化时，执行 `npm ci`。
- 前端源码、入口或构建配置变化时，执行前端构建。
- 输入未变化时，跳过依赖安装与前端构建。

构建时间戳保存在被忽略的 `frontend/node_modules` 和 `frontend/dist` 中。需要强制重新构建前端时，删除 `frontend/dist/.build-stamp` 后再次运行 Make。

## 发布

```sh
make release
```

产物为 `bin/market-mood-linux-amd64`，内嵌网页，服务器无需安装 Go 或 Node.js。部署步骤见 [服务器部署](../deploy/README.md)。
