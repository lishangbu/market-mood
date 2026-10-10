package server

import (
	"flag"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"
)

type Options struct{ Database, Bind string }

func Parse(args []string, getenv func(string) string, out io.Writer) (Options, error) {
	flags := flag.NewFlagSet("market-mood", flag.ContinueOnError)
	flags.SetOutput(out)
	directory := flags.String("data-dir", getenv("MARKET_MOOD_DATA_DIR"), "数据目录（默认当前工作目录）")
	database := flags.String("db", getenv("MARKET_MOOD_DB"), "SQLite数据库路径，优先于数据目录")
	address := getenv("MARKET_MOOD_BIND")
	if address == "" {
		address = "0.0.0.0:8080"
	}
	bind := flags.String("bind", address, "监听IP:端口")
	flags.Usage = func() {
		fmt.Fprintln(out, "市场情绪查看器 Go Web服务\n默认监听0.0.0.0:8080；Ctrl+C停止服务")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("未知启动参数：%s", flags.Arg(0))
	}
	if _, err := netip.ParseAddrPort(*bind); err != nil {
		return Options{}, fmt.Errorf("监听地址格式应为 IP:端口: %w", err)
	}
	db := *database
	if db == "" {
		db = filepath.Join(*directory, "market-mood.db")
	}
	return Options{Database: db, Bind: *bind}, nil
}
