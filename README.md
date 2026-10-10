<a id="readme-top"></a>

<div align="center">
  <h1>Market Mood</h1>
  <p>市场情绪查看器 · A 股看盘与复盘 Web 应用</p>
  <p>
    <a href="docs/requirements.md">产品说明</a> ·
    <a href="deploy/README.md">部署指南</a> ·
    <a href="https://github.com/lishangbu/market-mood/issues">报告问题</a> ·
    <a href="https://github.com/lishangbu/market-mood/issues">提出建议</a>
  </p>
</div>

<details>
  <summary>目录</summary>
  <ol>
    <li><a href="#项目介绍">项目介绍</a></li>
    <li><a href="#快速开始">快速开始</a></li>
    <li><a href="#使用说明">使用说明</a></li>
    <li><a href="#服务器部署">服务器部署</a></li>
    <li><a href="#验证与测试">验证与测试</a></li>
    <li><a href="#开发计划">开发计划</a></li>
    <li><a href="#参与贡献">参与贡献</a></li>
    <li><a href="#许可">许可</a></li>
    <li><a href="#联系">联系</a></li>
    <li><a href="#致谢">致谢</a></li>
  </ol>
</details>

## 项目介绍

Market Mood 面向个人 A 股市场观察，集中展示情绪评分、股票池、市场宽度和量能。后台独立采集行情，浏览器用于看盘、复盘和管理自选。

- 八因子容量情绪评分，展示分项、权重与计算口径。
- 昨日十亿首板、炸板、20cm 首板、主板首板及成交前50追踪。
- 当日主板封炸板、容量高度、涨跌与阴阳家数、昨同刻成交额比较。
- 股票名称或代码搜索、市场筛选、详情及持久化自选。
- 按日复盘、历史补录和采集记录保存。
- 手机优先的深色界面、红涨绿跌、可选语音提示。
- 来源状态、任务状态和数据时间可见；未知数据保持为空。

### 技术栈

| 层次 | 技术 |
| --- | --- |
| 后端 | Go 1.27、net/http、context、slog |
| 存储 | SQLite、modernc.org/sqlite |
| 前端 | React 19、TypeScript、Vite |
| 测试 | Go testing、Playwright |
| 部署 | Linux amd64 静态二进制、systemd |

前端资源内嵌到二进制，服务器运行无需 Go、Node.js 或 JavaScript 引擎。

## 快速开始

### 环境要求

构建需要 Go 1.27、Node.js 20.19+ 或 22.12+、npm、GNU Make，以及可访问行情接口的网络环境。

### 安装与运行

```sh
git clone https://github.com/lishangbu/market-mood.git
cd market-mood
make build
./bin/market-mood --bind 127.0.0.1:8080 --data-dir ./data
```

浏览器访问 [http://127.0.0.1:8080](http://127.0.0.1:8080)。首次运行会同步股票目录、交易日历和行情，并计算昨日股票池；完成前页面会显示进度或缺失状态。

### 前端开发

先按上述方式启动后端，再打开另一个终端：

```sh
cd frontend
npm run dev
```

Vite 将 `/api` 请求代理到 `127.0.0.1:8080`。前端源码改动后，发布前重新运行 `make build`。

## 使用说明

- **看盘**：查看情绪评分、市场宽度、量能和股票池；展开股票池进行搜索、排序或查看股票详情。
- **复盘**：按日期查看已有历史记录。盘中断档不补零、不插值；历史日线计算不代替盘中快照。
- **自选**：搜索股票并加入自选，刷新或重启后保留。
- **数据源设置**：选择行情来源，检查实际使用来源与回退原因。

页面显示交易日期和来源时间。休市时展示最近交易日行情；报价过期、来源失败或评分因子缺失时，不生成当前总评分。评分 v1 是初始观察指标，尚未经回测。

### 配置

| 参数 | 环境变量 | 默认值或行为 |
| --- | --- | --- |
| `--bind` | `MARKET_MOOD_BIND` | `0.0.0.0:8080` |
| `--data-dir` | `MARKET_MOOD_DATA_DIR` | 当前工作目录 |
| `--db` | `MARKET_MOOD_DB` | 显式数据库路径，优先于数据目录 |

`Ctrl+C` 或 `SIGTERM` 会停止采集并排空 HTTP 请求。开发环境按需手动启动服务，参见 [本机开发说明](docs/development.md)。

## 服务器部署

目标平台为 Linux amd64：

```sh
make release
./bin/market-mood-linux-amd64 --bind 127.0.0.1:8080 --data-dir ./data
```

发布产物是单个静态二进制。长期运行可使用仓库中的 systemd 服务，通过反向代理提供访问；安装、升级和停止方式见 [部署指南](deploy/README.md)。

## 验证与测试

```sh
make test
make vet
node --test scripts/audit-day.test.mjs

cd frontend
npx playwright install chromium
npm run test:e2e -- --workers=2
```

Make 会根据依赖和源码变化增量安装依赖、构建前端，连续运行测试和静态检查不会重复构建。已完成前端构建时，可直接运行 `CGO_ENABLED=0 go test ./...`。已有 Chrome 可通过 `MARKET_MOOD_BROWSER_CHANNEL=chrome` 使用，或设置 `MARKET_MOOD_BROWSER_PATH` 指定 Chromium 可执行文件。

真实数据源检查：

```sh
CGO_ENABLED=0 go run ./cmd/sourcecheck -timeout 35s
CGO_ENABLED=0 go run ./cmd/sourcecheck -catalog -timeout 4m
```

检查工具只读访问行情源，不打开应用数据库；任一来源失败会返回非零退出码。公共 TDX 节点可能返回无效数据，程序保持协议校验并使用有效备用来源。离线测试、真实来源检查和完整交易日审计验证不同范围，详细流程见 [验收说明](docs/validation/README.md)。

## 开发计划

- [x] 行情采集、股票目录、股票池与情绪评分。
- [x] 自选、历史复盘及手机界面。
- [x] Linux amd64 单文件构建与部署配置。
- [ ] 在目标服务器完成实际部署验证。
- [ ] 完成一个真实交易日的连续采集审计。
- [ ] 持续核验公共数据源的可用性与一致性。

产品规则见 [产品说明](docs/requirements.md)，代码模块和数据边界见 [数据源说明](docs/data-sources.md)。

## 参与贡献

通过 [Issues](https://github.com/lishangbu/market-mood/issues) 报告问题或提出需求。提交改动时，请说明具体行为、验证方式及影响范围；涉及数据源或指标的改动应包含对应测试。

1. Fork 仓库并创建功能分支。
2. 完成修改并运行相关检查。
3. 提交代码，发起 Pull Request。

## 许可

项目采用 [GNU AGPL v3](LICENSE)。第三方参考代码的许可文件保留在其所在目录。

## 联系

项目维护者：[lishangbu](https://github.com/lishangbu)。项目问题请提交到 [market-mood Issues](https://github.com/lishangbu/market-mood/issues)。

## 致谢

感谢以下开源项目及其维护者为本项目提供基础能力：

- [Go](https://go.dev/) 与 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)：后端服务和纯 Go SQLite 支持。
- [React](https://react.dev/)、[TypeScript](https://www.typescriptlang.org/) 与 [Vite](https://vite.dev/)：网页界面和前端构建。
- [Lucide](https://lucide.dev/)：界面图标。
- [Playwright](https://playwright.dev/)：浏览器自动化测试。
- [AKShare](https://github.com/akfamily/akshare)：新浪 KLC 数据解码的参考实现与测试对照，相关 MIT 许可随参考代码保留。

同时感谢腾讯财经、新浪财经、东方财富、同花顺，以及沪深北交易所和巨潮资讯提供可核验的行情与公司资料。数据接口的可用性和字段含义以各来源实际响应为准。

<p align="right"><a href="#readme-top">返回顶部</a></p>
