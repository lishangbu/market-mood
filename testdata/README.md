# 测试样本

这些文件仅用于测试，不写入应用行情数据库。

- 新浪、同花顺和东方财富样本采集于 2026-10-10，覆盖原始响应解析、日期校验及字段规范化。
- `sina-sh600000-golden.json` 包含 6,402 条日线预期结果；生成工具为 `scripts/generate-sina-golden.cjs`，参考解码器和授权文件位于 `scripts/reference`。
- TDX 原始响应样本位于 `docs/validation/2026-10-09-tdx-wire-probe.json`，用于验证异常报文拒绝逻辑。
- `turnover-synthetic-*.json` 和 `frontend/tests/fixtures/dashboard.json` 是合成测试场景，仅用于离线检查。

验证命令及范围见 [验收说明](../docs/validation/README.md)。
