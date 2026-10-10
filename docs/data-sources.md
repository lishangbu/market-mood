# 数据源与代码结构

应用以 Go Web 服务采集、校验及保存行情，React 页面读取本地 API。前端资源嵌入二进制，数据库采用纯 Go SQLite 驱动。

## 模块

| 模块 | 职责 |
| --- | --- |
| `internal/provider` | 公共行情接口、字段规范化、来源选择和响应校验 |
| `internal/catalog` | 股票目录及交易所资料核对、上市日期补充 |
| `internal/trading` | 上海时区、交易日历、交易时段和数据新鲜度 |
| `internal/market` | 股票池与情绪评分计算、统一字段契约 |
| `internal/turnover` | 沪深指数分钟成交额及昨同刻比较 |
| `internal/rebuild` | 按已核验交易日与日线计算昨日池、成交前50 |
| `internal/backfill` | 最近五个已结束交易日的历史补录 |
| `internal/store` | SQLite 事务、行情历史、配置、自选和运行状态 |
| `internal/tasks`、`internal/app` | 独立后台任务、退避、任务监督及协调 |
| `internal/httpapi`、`internal/server` | HTTP API、网页服务及正常退出 |
| `frontend/src` | 看盘、复盘、自选、股票详情及数据源设置 |

## 校验边界

- 来源解析统一价格为元、金额为元、盘口委托量为股。缺失字段保持未知，不填零。
- 报价按实际来源时间判定新鲜度，不将抓取时间当作报价时间。
- 名单分页、股票身份、交易日期、正文长度和数据完整性均须校验。来源失败保留旧数据并显示原因，不能用空列表冒充成功。
- 沪深上市日期优先使用交易所资料；缺失时读取股票身份一致的公司资料页及已核实公司公告，不推断日期。
- TDX 节点返回异常数据后暂时冷却，并按配置回退至其他有效来源。来源可用性以实际响应为准。
- 昨日池成员依据上一交易日数据固定，随后关联当前报价；前端搜索、排序不修改整池统计。
- 日线计算适用于普通10%/20%涨跌停规则，特殊交易规则样本按证据排除；不将日线结果当作盘中快照。
- 指数成交额与股票池筛选独立。昨同刻比较要求精确同分钟匹配，不插值或以邻近分钟替代。
- 股票池缓存必须与当前目录候选总数一致。未完成或存在失败的成交前50计算不发布排名。
- 休市期间可显示最近交易日数据；跨日获取的历史专题池仍须确认交易日期。来源失败、旧交易日或盘中超时会阻止当前总评分。

## 检查命令

```sh
make build
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go run ./cmd/sourcecheck -timeout 35s
CGO_ENABLED=0 go run ./cmd/sourcecheck -catalog -timeout 4m
node --test scripts/audit-day.test.mjs
npm --prefix frontend run test:e2e -- --workers=2
```

真实来源检查、离线测试与完整交易日审计分别验证不同范围，不能互相替代。详细命令见 [验收说明](validation/README.md)。
