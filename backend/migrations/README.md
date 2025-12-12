# 数据库迁移指南

本目录包含数据库迁移脚本，用于管理数据库结构变更。

## 迁移脚本列表

### 20241212_drop_deprecated_life_tables.go

**目的**: 删除废弃的生命探针相关表

**删除的表**:
- `life_focus_events` - 专注状态记录
- `life_screen_events` - 屏幕使用记录
- `life_energy_samples` - 能量值详情
- `life_energy_daily_totals` - 每日能量汇总

**背景**: 根据产品迭代需求，将生命探针数据类型从 6 种精简为 3 种（心率、步数、睡眠），废弃能量、专注和屏幕使用相关功能。

## 使用方法

### 方法 1: 交互式运行（推荐）

```bash
cd backend/migrations
go run 20241212_drop_deprecated_life_tables.go [数据库路径]
```

脚本会显示迁移信息并等待确认：
- 输入 `yes` 或 `y` 继续执行
- 输入其他内容取消迁移

示例：
```bash
# 使用默认路径 ./data/monitor.db
go run 20241212_drop_deprecated_life_tables.go

# 指定数据库路径
go run 20241212_drop_deprecated_life_tables.go /path/to/your/database.db
```

### 方法 2: 自动化运行

```bash
export AUTO_MIGRATE=true
export DB_PATH=/path/to/database.db
go run 20241212_drop_deprecated_life_tables.go
```

### 方法 3: 编译后运行

```bash
cd backend/migrations
go build -o migrate_drop_tables 20241212_drop_deprecated_life_tables.go
./migrate_drop_tables [数据库路径]
```

## 迁移前准备

### 1. 备份数据库

**强烈建议在执行迁移前备份数据库！**

```bash
# 备份数据库文件
cp ./data/monitor.db ./data/monitor.db.backup.$(date +%Y%m%d_%H%M%S)
```

### 2. 检查数据

如果需要保留废弃表中的数据，请先导出：

```bash
sqlite3 ./data/monitor.db <<EOF
.headers on
.mode csv
.output life_focus_events_backup.csv
SELECT * FROM life_focus_events;
.output life_screen_events_backup.csv
SELECT * FROM life_screen_events;
.output stdout
EOF
```

### 3. 确认环境

确保数据库文件存在且有写权限：
```bash
ls -lh ./data/monitor.db
```

## 迁移后验证

迁移脚本执行完成后会自动显示：
1. 各表的删除状态
2. 剩余表的记录数统计
3. 数据库文件大小

你也可以手动验证：

```bash
# 检查表是否已删除
sqlite3 ./data/monitor.db "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'life_%';"

# 查看数据库大小
du -h ./data/monitor.db
```

## 回滚方案

⚠️ **此迁移不可逆**

如果需要恢复，只能通过备份文件恢复：

```bash
# 停止应用
pkill -f server-ops-backend

# 恢复备份
cp ./data/monitor.db.backup.YYYYMMDD_HHMMSS ./data/monitor.db

# 重启应用
./server-ops-backend
```

## 故障排查

### 问题: 数据库文件不存在

**错误信息**: `数据库文件不存在: ./data/monitor.db`

**解决方案**: 确认数据库文件路径，或指定正确的路径：
```bash
go run 20241212_drop_deprecated_life_tables.go /path/to/monitor.db
```

### 问题: 权限不足

**错误信息**: `unable to open database file`

**解决方案**: 检查文件权限
```bash
chmod 644 ./data/monitor.db
```

### 问题: 数据库被锁定

**错误信息**: `database is locked`

**解决方案**: 停止正在运行的应用
```bash
pkill -f server-ops-backend
# 然后重新运行迁移
```

## 技术细节

- **数据库类型**: SQLite
- **ORM 框架**: GORM v2
- **迁移策略**: 表级删除（DROP TABLE）
- **安全机制**:
  - 交互式确认
  - 表存在性检查
  - 详细日志输出
  - 统计信息展示

## 最佳实践

1. **始终在非生产环境先测试**
2. **备份数据库文件**
3. **在低峰期执行迁移**
4. **保留迁移日志**
5. **验证迁移结果**

## 相关文档

- [系统设计文档](../../docs/life_probe_migration.md)
- [API 变更说明](../../docs/api_changes.md)
- [数据保留策略](../../docs/data_retention.md)
