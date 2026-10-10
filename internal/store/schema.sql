-- Explicit typed tables; unknown measurements are NULL. No content-addressed
-- entities or garbage collector. Historical records will own their observations.
CREATE TABLE watchlist (
 code TEXT PRIMARY KEY CHECK(length(code)=6), name TEXT NOT NULL, created_at TEXT NOT NULL
) STRICT;
CREATE TABLE source_config (
 capability TEXT NOT NULL CHECK(capability IN ('quotes','stock_quotes')),
 position INTEGER NOT NULL CHECK(position>=0), source TEXT NOT NULL,
 PRIMARY KEY(capability,position), UNIQUE(capability,source)
) STRICT;
CREATE TABLE securities (
 code TEXT PRIMARY KEY, name TEXT NOT NULL, exchange TEXT NOT NULL,
 board TEXT NOT NULL, industry TEXT, listing_date TEXT, listing_status TEXT, provider TEXT NOT NULL
) STRICT;
CREATE INDEX securities_exchange_code ON securities(exchange,code);
CREATE TABLE catalog_status (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), updated_at TEXT, attempted_at TEXT, provider TEXT, error TEXT
) STRICT;
INSERT INTO catalog_status(singleton) VALUES(1);
CREATE TABLE catalog_fallbacks (position INTEGER PRIMARY KEY, reason TEXT NOT NULL) STRICT;
CREATE TABLE catalog_exchanges (exchange TEXT PRIMARY KEY, count INTEGER NOT NULL, check_result TEXT) STRICT;
CREATE TABLE daily_bars (
 code TEXT NOT NULL, day TEXT NOT NULL, source TEXT NOT NULL,
 open REAL NOT NULL, close REAL NOT NULL, high REAL NOT NULL, low REAL NOT NULL,
 volume REAL NOT NULL, amount REAL, prevclose REAL,
 PRIMARY KEY(code,day,source)
) STRICT;
CREATE TABLE turnover_state (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), provider TEXT NOT NULL, captured_at TEXT, error TEXT
) STRICT;
CREATE TABLE turnover_minutes (
 day TEXT NOT NULL, minute TEXT NOT NULL, amount REAL NOT NULL CHECK(amount>=0),
 PRIMARY KEY(day,minute)
) STRICT;
-- Only dates in the latest provider window participate in current comparisons.
-- Retained older dates must never silently fill holes in a new source window.
CREATE TABLE turnover_active_days (day TEXT PRIMARY KEY) STRICT;
CREATE TABLE catalog_revisions (
 id INTEGER PRIMARY KEY, captured_at TEXT NOT NULL, provider TEXT NOT NULL
) STRICT;
CREATE TABLE catalog_revision_securities (
 revision INTEGER NOT NULL REFERENCES catalog_revisions(id),
 code TEXT NOT NULL, name TEXT NOT NULL, exchange TEXT NOT NULL, board TEXT NOT NULL,
 industry TEXT, listing_date TEXT, listing_status TEXT, provider TEXT NOT NULL,
 PRIMARY KEY(revision,code)
) STRICT;
CREATE TABLE catalog_revision_checks (
 revision INTEGER NOT NULL REFERENCES catalog_revisions(id),
 exchange TEXT NOT NULL, count INTEGER NOT NULL, check_result TEXT,
 PRIMARY KEY(revision,exchange)
) STRICT;
CREATE TABLE catalog_revision_fallbacks (
 revision INTEGER NOT NULL REFERENCES catalog_revisions(id), position INTEGER NOT NULL,
 reason TEXT NOT NULL, PRIMARY KEY(revision,position)
) STRICT;

-- Each set owns a full copy of its quotes. History never dereferences mutable
-- catalog or current quote rows, and no hash-based object graph is involved.
CREATE TABLE stock_sets (id INTEGER PRIMARY KEY, row_count INTEGER NOT NULL CHECK(row_count>=0)) STRICT;
CREATE TABLE stock_set_rows (
 set_id INTEGER NOT NULL REFERENCES stock_sets(id), position INTEGER NOT NULL,
 code TEXT NOT NULL,
 name TEXT NOT NULL,
 industry TEXT NOT NULL,
 price REAL,
 change REAL,
 amount REAL,
 seal_amount REAL,
 break_count INTEGER,
 float_cap REAL,
 market_cap REAL,
 turnover_rate REAL,
 first_seal_time TEXT,
 last_seal_time TEXT,
 open REAL,
 previous_close REAL,
 source_time TEXT,
 provider TEXT,
 limit_up REAL,
 limit_down REAL,
 boards INTEGER,
 sealed INTEGER,
 eligible INTEGER NOT NULL,
 stats_days INTEGER,
 stats_count INTEGER,
 PRIMARY KEY(set_id,position), UNIQUE(set_id,code)
) STRICT;
CREATE TABLE stock_set_books (
 set_id INTEGER NOT NULL, stock_position INTEGER NOT NULL, side TEXT NOT NULL CHECK(side IN ('bid','ask')),
 position INTEGER NOT NULL, price REAL NOT NULL, shares REAL NOT NULL,
 PRIMARY KEY(set_id,stock_position,side,position),
 FOREIGN KEY(set_id,stock_position) REFERENCES stock_set_rows(set_id,position)
) STRICT;
CREATE TABLE stock_set_fallbacks (
 set_id INTEGER NOT NULL, stock_position INTEGER NOT NULL, position INTEGER NOT NULL, reason TEXT NOT NULL,
 PRIMARY KEY(set_id,stock_position,position),
 FOREIGN KEY(set_id,stock_position) REFERENCES stock_set_rows(set_id,position)
) STRICT;
CREATE TABLE market_frames (
 id INTEGER PRIMARY KEY, day TEXT NOT NULL, captured_at TEXT NOT NULL,
 source_time TEXT, provider TEXT, complete INTEGER NOT NULL CHECK(complete IN (0,1)),
 stocks INTEGER NOT NULL REFERENCES stock_sets(id)
) STRICT;
CREATE TABLE market_frame_fallbacks (
 frame INTEGER NOT NULL REFERENCES market_frames(id), position INTEGER NOT NULL,
 reason TEXT NOT NULL, PRIMARY KEY(frame,position)
) STRICT;
CREATE TABLE limit_frames (
 id INTEGER PRIMARY KEY, day TEXT NOT NULL, captured_at TEXT NOT NULL,
 sealed INTEGER NOT NULL REFERENCES stock_sets(id), broken INTEGER NOT NULL REFERENCES stock_sets(id)
) STRICT;
CREATE TABLE runtime_state (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 market_frame INTEGER REFERENCES market_frames(id), limit_frame INTEGER REFERENCES limit_frames(id),
 watch_stocks INTEGER REFERENCES stock_sets(id), market_error TEXT, limit_error TEXT,
 calendar_error TEXT, watch_error TEXT
) STRICT;
INSERT INTO runtime_state(singleton) VALUES(1);
CREATE TABLE calendar_days (day TEXT PRIMARY KEY) STRICT;
CREATE TABLE top50_states (
 id INTEGER PRIMARY KEY, version INTEGER NOT NULL, day TEXT NOT NULL,
 checked INTEGER NOT NULL, total INTEGER NOT NULL, excluded INTEGER NOT NULL, failed INTEGER NOT NULL,
 complete INTEGER NOT NULL, running INTEGER NOT NULL, error TEXT, updated_at TEXT NOT NULL,
 stocks INTEGER NOT NULL REFERENCES stock_sets(id)
) STRICT;
CREATE TABLE yesterday_states (
 id INTEGER PRIMARY KEY, version INTEGER NOT NULL, day TEXT NOT NULL,
 checked INTEGER NOT NULL, total INTEGER NOT NULL, excluded INTEGER NOT NULL, failed INTEGER NOT NULL,
 complete INTEGER NOT NULL, running INTEGER NOT NULL, error TEXT, updated_at TEXT NOT NULL,
 limits INTEGER REFERENCES limit_frames(id), tdx_count INTEGER NOT NULL
) STRICT;
CREATE TABLE rebuild_current (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 top50_state INTEGER REFERENCES top50_states(id), yesterday_state INTEGER REFERENCES yesterday_states(id)
) STRICT;
INSERT INTO rebuild_current(singleton) VALUES(1);

CREATE TABLE breadth_records (id INTEGER PRIMARY KEY,
 up INTEGER NOT NULL,
 down INTEGER NOT NULL,
 flat INTEGER NOT NULL,
 yang INTEGER NOT NULL,
 yin INTEGER NOT NULL,
 valid INTEGER NOT NULL,
 excluded INTEGER NOT NULL,
 amount REAL,
 previous_same_amount REAL,
 volume_change REAL,
 forecast REAL
) STRICT;

CREATE TABLE sealing_records (id INTEGER PRIMARY KEY,
 sealed INTEGER NOT NULL,
 broken INTEGER NOT NULL,
 rate REAL,
 height INTEGER
) STRICT;

CREATE TABLE turnover_views (id INTEGER PRIMARY KEY,
 amount REAL,
 previous_same_amount REAL,
 volume_change REAL,
 forecast REAL,
 source_time TEXT,
 previous_day TEXT,
 provider TEXT NOT NULL,
 captured_at TEXT,
 stale INTEGER NOT NULL,
 error TEXT
) STRICT;
CREATE TABLE dashboards (
 id INTEGER PRIMARY KEY, day TEXT, generated_at TEXT NOT NULL, observed_at TEXT,
 session TEXT NOT NULL, previous_day TEXT, score_version TEXT NOT NULL, score_total REAL,
 breadth INTEGER REFERENCES breadth_records(id), capacity INTEGER REFERENCES sealing_records(id),
 sealing INTEGER REFERENCES sealing_records(id), turnover INTEGER REFERENCES turnover_views(id),
 top50 INTEGER REFERENCES top50_states(id), yesterday INTEGER REFERENCES yesterday_states(id),
 pool_count INTEGER NOT NULL, factor_count INTEGER NOT NULL, source_count INTEGER NOT NULL, note_count INTEGER NOT NULL
) STRICT;
CREATE TABLE dashboard_pools (
 dashboard INTEGER NOT NULL REFERENCES dashboards(id), position INTEGER NOT NULL,
 id TEXT NOT NULL,title TEXT NOT NULL,description TEXT NOT NULL,ready INTEGER NOT NULL,
 count INTEGER NOT NULL,valid_count INTEGER NOT NULL,average REAL,yang_rate REAL,
 stocks INTEGER NOT NULL REFERENCES stock_sets(id),PRIMARY KEY(dashboard,position)
) STRICT;
CREATE TABLE dashboard_factors (
 dashboard INTEGER NOT NULL REFERENCES dashboards(id), position INTEGER NOT NULL,
 name TEXT NOT NULL,weight REAL NOT NULL,value REAL,score REAL,unit TEXT NOT NULL,
 range_low REAL NOT NULL,range_high REAL NOT NULL,PRIMARY KEY(dashboard,position)
) STRICT;
CREATE TABLE dashboard_sources (
 dashboard INTEGER NOT NULL REFERENCES dashboards(id),position INTEGER NOT NULL,
 name TEXT NOT NULL,captured_at TEXT,source_time TEXT,provider TEXT,error TEXT,
 stale INTEGER NOT NULL,fallback_count INTEGER NOT NULL,PRIMARY KEY(dashboard,position)
) STRICT;
CREATE TABLE dashboard_source_fallbacks (
 dashboard INTEGER NOT NULL,source_position INTEGER NOT NULL,position INTEGER NOT NULL,
 reason TEXT NOT NULL,PRIMARY KEY(dashboard,source_position,position),
 FOREIGN KEY(dashboard,source_position) REFERENCES dashboard_sources(dashboard,position)
) STRICT;
CREATE TABLE dashboard_notes (
 dashboard INTEGER NOT NULL REFERENCES dashboards(id),position INTEGER NOT NULL,
 note TEXT NOT NULL,PRIMARY KEY(dashboard,position)
) STRICT;
CREATE TABLE snapshots (
 day TEXT NOT NULL, minute TEXT NOT NULL, dashboard INTEGER NOT NULL REFERENCES dashboards(id),
 PRIMARY KEY(day,minute)
) STRICT;
CREATE TABLE closes (
 day TEXT PRIMARY KEY,market INTEGER NOT NULL REFERENCES market_frames(id),
 limits INTEGER NOT NULL REFERENCES limit_frames(id),dashboard INTEGER NOT NULL REFERENCES dashboards(id)
) STRICT;
CREATE TABLE historical_days (
 day TEXT PRIMARY KEY, fields_version INTEGER NOT NULL, imported_at TEXT NOT NULL, source TEXT NOT NULL,
 sh_open REAL NOT NULL,sh_close REAL NOT NULL,sh_high REAL NOT NULL,sh_low REAL NOT NULL,sh_volume REAL NOT NULL,
 sz_open REAL NOT NULL,sz_close REAL NOT NULL,sz_high REAL NOT NULL,sz_low REAL NOT NULL,sz_volume REAL NOT NULL,
 limits INTEGER REFERENCES limit_frames(id),limitation TEXT
) STRICT;
CREATE TABLE backfill_status (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),completed INTEGER NOT NULL,running INTEGER NOT NULL,error TEXT,updated_at TEXT NOT NULL
) STRICT;
INSERT INTO backfill_status VALUES(1,0,0,NULL,'');
CREATE TABLE backfill_targets(position INTEGER PRIMARY KEY,day TEXT NOT NULL UNIQUE) STRICT;
