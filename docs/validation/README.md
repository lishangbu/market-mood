# 验收证据

固定样本、真实启动和完整交易日验证分别记录。任何固定样本的通过都不能替代真实来源或完整交易日证据。

## 重复执行

先 `make build`。下列Node脚本仅是仓库中的开发验收工具，不属于Go服务运行时：

```sh
# 测试审计器自身，不证明真实行情通过
node --test scripts/audit-day.test.mjs
# 独立端口/数据库，持续采集并每分钟记录只读证据
node scripts/acceptance-session.mjs /tmp/market-mood-acceptance ./bin/market-mood
# 已运行服务的单次观测
node scripts/observe.mjs http://127.0.0.1:18080 /tmp/market-mood-observations --once
# 指定日的只读审计；非零退出码代表未通过
node scripts/audit-day.mjs 2026-10-09 http://127.0.0.1:18080 /tmp/market-mood-day-audit.json
```

持续验收应在开盘前启动并保持运行至15:10以后。`--once`只验证启动及单次响应，不能证明整日完整。

审计要求至少95%交易分钟覆盖、最长缺口不超3分钟、开收盘窗口有观测、有效收盘快照，以及来源、评分求和、封板率和涨跌/阴阳家数一致。未知分数保留为空，因此审计器通过也不能单独证明昨日池重建成功；还必须确认重建完整状态和真实来源检查。

完整交易日审计需要真实交易时段的完整记录，不能由短时间运行或离线样本证明。
## 二进制启动与持久化检查

```sh
python3 scripts/smoke-binary.py bin/market-mood-linux-amd64 --report /tmp/amd64-smoke.json
```

脚本为每次检查创建临时数据库，真实启动二进制两次，校验内嵌首页、十类任务、看板、数据库监控、配置修改及重启保留，并发送SIGTERM验证正常退出。它会启动正常来源采集，但不以短时间内的HTTP成功证明数据源完整性。服务器目标为 Linux amd64，部署后应在目标主机执行启动和数据源检查。
