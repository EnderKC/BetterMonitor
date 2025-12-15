# 生命探针（LifeLogger）技术文档

## 目录

1. [概览](#概览)
2. [快速接入](#快速接入)
3. [系统架构](#系统架构)
4. [API 规范](#api-规范)
5. [数据上传格式规范](#数据上传格式规范)
6. [数据库存储结构](#数据库存储结构)
7. [前端展示逻辑](#前端展示逻辑)
8. [第三方适配指南](#第三方适配指南)
9. [附录](#附录)

---

## 概览

### 系统目标

生命探针（LifeLogger）是一个健康数据采集与可视化系统，用于收集、存储和展示来自可穿戴设备或健康监测应用的生理数据。系统支持：

- 实时心率监测
- 步数与活动量追踪
- 睡眠质量分析

### 核心概念

- **探针（LifeProbe）**：代表一个数据源设备或监测点，每个探针有唯一的 `device_id`
- **事件（LifeLoggerEvent）**：设备上传的原始数据包，包含时间戳、数据类型和载荷
- **样本（Sample）**：经过解析的具体数据点，如心率测量值或步数区间
- **分段（Segment）**：时间区间数据，如睡眠阶段

### 支持的数据类型

| 数据类型 | 标识符 | 描述 |
|---------|--------|------|
| 心率数据 | `heart_rate` | 实时或周期性心率测量值（BPM） |
| 步数详细数据 | `steps_detailed` | 带时间区间的步数样本 |
| 睡眠详细数据 | `sleep_detailed` | 睡眠阶段分段数据 |

---

## 快速接入

### 接入前置条件

1. **创建生命探针**
   - 通过管理员界面创建探针，获取 `device_id`
   - 探针创建后，记录返回的 `device_id`，客户端需要使用此 ID 上传数据

2. **无需身份认证**
   - 数据上传接口 `/api/life-logger/events` 不需要 JWT Token
   - 仅需提供正确的 `device_id` 即可上传数据

### 最短接入流程

#### 1. 创建探针（管理员操作）

```bash
POST /api/life-probes
Authorization: Bearer <admin_token>
Content-Type: application/json

{
  "name": "张三的 iPhone",
  "device_id": "iphone-12345-abcde",
  "description": "iPhone 14 Pro + Apple Watch",
  "tags": "Apple, Watch",
  "allow_public_view": true
}
```

#### 2. 上传心率数据

```bash
POST /api/life-logger/events
Content-Type: application/json

{
  "event_id": "hr-2025-01-15-001",
  "device_id": "iphone-12345-abcde",
  "timestamp": "2025-01-15T10:30:00Z",
  "data_type": "heart_rate",
  "battery_level": 0.85,
  "payload": {
    "value": 72,
    "unit": "bpm",
    "measure_time": "2025-01-15T10:30:00Z"
  }
}
```

#### 3. 验证数据

访问前端页面查看探针详情：
- 管理员：`/admin/life-probes/{probe_id}`
- 公开访问：`/life-probes/{probe_id}` （如果探针设置为公开）

---

## 系统架构

### 组件图

```
┌─────────────────┐
│  第三方客户端   │
│ (LifeLogger App)│
└────────┬────────┘
         │ HTTP POST
         ▼
┌─────────────────────────────────────────┐
│         后端 API Server (Go)             │
│  ┌─────────────────────────────────┐   │
│  │ /api/life-logger/events         │   │
│  │  - 数据校验                      │   │
│  │  - 解析 Payload                 │   │
│  │  - 事务写入                      │   │
│  └────────────┬────────────────────┘   │
└───────────────┼─────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────┐
│       数据库 (SQLite/PostgreSQL)        │
│  - life_probes                          │
│  - life_logger_events (原始事件)        │
│  - life_heart_rates                     │
│  - life_step_samples                    │
│  - life_step_daily_totals               │
│  - life_sleep_segments                  │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│        WebSocket 推送服务                │
│  - 探针列表实时更新                      │
│  - 探针详情实时更新                      │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│         前端 (Vue 3 + TypeScript)       │
│  - 探针列表页                            │
│  - 探针详情页（图表可视化）               │
└─────────────────────────────────────────┘
```

### 数据流时序

```
客户端              API Server              数据库              WebSocket
  │                    │                     │                     │
  ├─POST /events──────▶│                     │                     │
  │                    ├─校验 device_id──────▶│                     │
  │                    │◀────probe 信息──────┤                     │
  │                    ├─解析 payload        │                     │
  │                    ├─开始事务────────────▶│                     │
  │                    │  - 插入 event       │                     │
  │                    │  - 插入 samples     │                     │
  │                    │  - 更新 probe       │                     │
  │                    │◀────提交成功────────┤                     │
  │                    ├─通知更新───────────────────────────────────▶│
  │◀───200 OK──────────┤                     │                     │
  │                    │                     │                     ├─推送新数据▶前端
```

### 关键设计

- **幂等性**：通过 `event_id` 唯一索引保证，重复上传会被忽略（数据库返回唯一约束错误时静默成功）
- **事务性**：所有数据写入在单个数据库事务中完成，保证一致性
- **实时性**：写入成功后立即通过 WebSocket 推送更新
- **公开访问**：支持探针级别的公开展示，可脱敏设备 ID

---

## API 规范

### 数据采集接口

#### `POST /api/life-logger/events`

上传生命数据事件。

**请求头**
```
Content-Type: application/json
```

**请求体结构**

```json
{
  "event_id": "string (required, unique)",
  "device_id": "string (required)",
  "timestamp": "string (required, RFC3339 format)",
  "data_type": "string (required, enum: heart_rate|steps_detailed|sleep_detailed)",
  "battery_level": "float (optional, 0.0-1.0)",
  "payload": "object (required, 根据 data_type 不同)"
}
```

**字段说明**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `event_id` | string | ✓ | 事件唯一标识符，由客户端生成。建议格式：`{type}-{timestamp}-{seq}` |
| `device_id` | string | ✓ | 探针设备 ID，必须在系统中已创建 |
| `timestamp` | string | ✓ | 事件时间戳，RFC3339 格式（UTC），如 `2025-01-15T10:30:00Z` |
| `data_type` | string | ✓ | 数据类型标识符 |
| `battery_level` | float | - | 设备电量（0.0-1.0），如 0.85 表示 85% |
| `payload` | object | ✓ | 数据载荷，结构取决于 `data_type` |

**响应**

- **成功**：`200 OK`
  ```json
  {
    "success": true
  }
  ```

- **失败**：`4xx/5xx`
  ```json
  {
    "error": "错误描述"
  }
  ```

**错误码**

| HTTP 状态码 | 错误原因 |
|------------|---------|
| 400 | 请求体格式错误、缺少必填字段、数据类型不支持 |
| 404 | 探针不存在（device_id 未注册） |
| 500 | 服务器内部错误 |

#### 测试接口（Ping）

发送 ping 请求测试探针是否存在：

```json
POST /api/life-logger/events
{
  "ping": "test",
  "device_id": "your-device-id",
  "timestamp": "2025-01-15T10:00:00Z"
}
```

**响应**
```json
{
  "success": true,
  "message": "pong",
  "device_id": "your-device-id",
  "probe_name": "探针名称",
  "allow_public_view": true,
  "received_at": "2025-01-15T10:00:01.234Z"
}
```

---

## 数据上传格式规范

### 统一约束

#### 时间格式
- **格式**：RFC3339（ISO 8601）
- **时区**：必须使用 UTC 时区（`Z` 后缀）或明确指定偏移量（`+08:00`）
- **示例**：
  - ✓ `2025-01-15T10:30:00Z`
  - ✓ `2025-01-15T18:30:00+08:00`
  - ✗ `2025-01-15 10:30:00`（缺少 T 分隔符和时区）

#### 时间语义
- `timestamp`：事件上传时间或数据生成时间
- `measure_time`：具体测量时刻（心率）
- `start_time` / `end_time`：区间数据的开始和结束时间（步数、睡眠）

#### 幂等性保证
- 使用 `event_id` 作为幂等键
- 相同 `event_id` 的重复上传会被静默忽略（返回 200 OK）
- 建议 `event_id` 格式：`{data_type}-{date}-{sequence}`
  - 示例：`heart_rate-2025-01-15-001`

---

### 1. 心率数据（heart_rate）

#### Payload 结构

```json
{
  "value": 72.0,
  "unit": "bpm",
  "measure_time": "2025-01-15T10:30:00Z"
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `value` | float | ✓ | 心率值，单位为 BPM |
| `unit` | string | ✓ | 单位标识，固定为 `"bpm"` |
| `measure_time` | string | ✓ | 测量时间（RFC3339） |

#### 约束与建议

- **数值范围**：建议 40-200 BPM，异常值会被保留但可能影响展示
- **采样频率**：建议每分钟或按需上传，避免过于频繁（如每秒）
- **异常处理**：`NaN` 或 `Inf` 值会被规范化为 0

#### 完整示例

```json
{
  "event_id": "hr-2025-01-15-10-30-00",
  "device_id": "iphone-12345",
  "timestamp": "2025-01-15T10:30:05Z",
  "data_type": "heart_rate",
  "battery_level": 0.85,
  "payload": {
    "value": 72,
    "unit": "bpm",
    "measure_time": "2025-01-15T10:30:00Z"
  }
}
```

---

### 2. 步数详细数据（steps_detailed）

#### Payload 结构

```json
{
  "start_period": "2025-01-15T00:00:00Z",
  "end_period": "2025-01-15T23:59:59Z",
  "samples": [
    {
      "value": 120,
      "start_time": "2025-01-15T08:00:00Z",
      "end_time": "2025-01-15T08:15:00Z"
    },
    {
      "value": 80,
      "start_time": "2025-01-15T08:15:00Z",
      "end_time": "2025-01-15T08:30:00Z"
    }
  ],
  "total_count": 200,
  "today_total_steps": 5432
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `start_period` | string | ✓ | 数据窗口起始时间 |
| `end_period` | string | ✓ | 数据窗口结束时间 |
| `samples` | array | ✓ | 步数样本数组 |
| `samples[].value` | float | ✓ | 区间内的步数 |
| `samples[].start_time` | string | ✓ | 区间开始时间 |
| `samples[].end_time` | string | ✓ | 区间结束时间 |
| `total_count` | float | - | 本次上传样本的步数总和 |
| `today_total_steps` | float | - | 当天累计总步数（用于覆盖聚合值） |

#### 约束与建议

- **区间语义**：`value` 表示 `[start_time, end_time)` 区间内的累计步数
- **区间要求**：
  - 必须满足 `start_time < end_time`
  - 允许区间重叠（后端会去重处理）
  - 允许跨天区间（会按天拆分计算）
- **聚合逻辑**：
  - 后端会自动将样本按天累加到 `life_step_daily_totals` 表
  - 如果提供 `today_total_steps`，会覆盖当天的聚合值（优先级更高）
- **上传策略**：
  - 增量上传：每次上传新增的样本（推荐）
  - 全量覆盖：提供 `today_total_steps` 确保准确性

#### 完整示例

```json
{
  "event_id": "steps-2025-01-15-001",
  "device_id": "iphone-12345",
  "timestamp": "2025-01-15T12:00:00Z",
  "data_type": "steps_detailed",
  "battery_level": 0.80,
  "payload": {
    "start_period": "2025-01-15T00:00:00Z",
    "end_period": "2025-01-15T12:00:00Z",
    "samples": [
      {
        "value": 1200,
        "start_time": "2025-01-15T08:00:00Z",
        "end_time": "2025-01-15T09:00:00Z"
      },
      {
        "value": 800,
        "start_time": "2025-01-15T09:00:00Z",
        "end_time": "2025-01-15T10:00:00Z"
      },
      {
        "value": 1500,
        "start_time": "2025-01-15T10:00:00Z",
        "end_time": "2025-01-15T11:00:00Z"
      }
    ],
    "total_count": 3500,
    "today_total_steps": 5432
  }
}
```

---

### 3. 睡眠详细数据（sleep_detailed）

#### Payload 结构

```json
{
  "is_sleep_session_finished": true,
  "segments": [
    {
      "stage": "deep",
      "start_time": "2025-01-15T00:30:00Z",
      "end_time": "2025-01-15T02:00:00Z",
      "duration": 5400
    },
    {
      "stage": "core",
      "start_time": "2025-01-15T02:00:00Z",
      "end_time": "2025-01-15T04:30:00Z",
      "duration": 9000
    },
    {
      "stage": "rem",
      "start_time": "2025-01-15T04:30:00Z",
      "end_time": "2025-01-15T06:00:00Z",
      "duration": 5400
    },
    {
      "stage": "awake",
      "start_time": "2025-01-15T06:00:00Z",
      "end_time": "2025-01-15T06:15:00Z",
      "duration": 900
    }
  ],
  "today_total_sleep_hours": 7.5
}
```

#### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `is_sleep_session_finished` | bool | ✓ | 睡眠会话是否结束 |
| `segments` | array | - | 睡眠阶段数组（可为空） |
| `segments[].stage` | string | ✓ | 睡眠阶段标识符 |
| `segments[].start_time` | string | ✓ | 阶段开始时间 |
| `segments[].end_time` | string | ✓ | 阶段结束时间 |
| `segments[].duration` | float | ✓ | 阶段持续时长（秒） |
| `today_total_sleep_hours` | float | - | 当天总睡眠时长（小时） |

#### 睡眠阶段枚举

| 标识符 | 名称 | 描述 | 前端显示颜色 |
|--------|------|------|--------------|
| `deep` | 深睡 | 深度睡眠阶段 | 紫色 `#722ed1` |
| `core` | 浅睡 | 浅度睡眠/核心睡眠 | 蓝色 `#1677ff` |
| `rem` | REM | 快速眼动睡眠 | 绿色 `#52c41a` |
| `awake` | 清醒 | 清醒或短暂醒来 | 橙色 `#faad14` |
| `in_bed` | 在床 | 在床但未入睡（不计入睡眠时长） | 灰色（不统计） |

#### 约束与建议

- **阶段要求**：
  - 必须满足 `start_time < end_time`
  - `duration` 应等于 `end_time - start_time` 的秒数（前端以此为准）
  - 允许阶段重叠或间隙（设备限制）
- **空阶段处理**：
  - 如果设备无法获取睡眠阶段数据，`segments` 可为空数组 `[]`
  - 这种情况下前端会显示"暂无睡眠阶段数据"
- **统计规则**：
  - `in_bed` 阶段不计入总睡眠时长（仅用于记录在床时间）
  - 总睡眠时长 = `deep` + `core` + `rem` + `awake` 的时长总和（排除 `in_bed`）

#### 第三方设备映射

不同设备的睡眠阶段命名可能不同，建议映射如下：

| 设备来源 | 原始阶段 | 映射到本系统 |
|---------|---------|-------------|
| Apple Health | Deep Sleep | `deep` |
| Apple Health | Core Sleep | `core` |
| Apple Health | REM Sleep | `rem` |
| Apple Health | Awake | `awake` |
| Apple Health | In Bed | `in_bed` |
| Fitbit | Deep | `deep` |
| Fitbit | Light | `core` |
| Fitbit | REM | `rem` |
| Fitbit | Wake | `awake` |
| Garmin | Deep | `deep` |
| Garmin | Light | `core` |
| Garmin | REM | `rem` |
| Mi Band | Deep Sleep | `deep` |
| Mi Band | Light Sleep | `core` |

#### 完整示例

```json
{
  "event_id": "sleep-2025-01-15",
  "device_id": "iphone-12345",
  "timestamp": "2025-01-15T07:00:00Z",
  "data_type": "sleep_detailed",
  "battery_level": 0.90,
  "payload": {
    "is_sleep_session_finished": true,
    "segments": [
      {
        "stage": "in_bed",
        "start_time": "2025-01-14T23:00:00Z",
        "end_time": "2025-01-14T23:15:00Z",
        "duration": 900
      },
      {
        "stage": "core",
        "start_time": "2025-01-14T23:15:00Z",
        "end_time": "2025-01-15T00:30:00Z",
        "duration": 4500
      },
      {
        "stage": "deep",
        "start_time": "2025-01-15T00:30:00Z",
        "end_time": "2025-01-15T02:00:00Z",
        "duration": 5400
      },
      {
        "stage": "rem",
        "start_time": "2025-01-15T02:00:00Z",
        "end_time": "2025-01-15T03:30:00Z",
        "duration": 5400
      },
      {
        "stage": "core",
        "start_time": "2025-01-15T03:30:00Z",
        "end_time": "2025-01-15T05:00:00Z",
        "duration": 5400
      },
      {
        "stage": "awake",
        "start_time": "2025-01-15T05:00:00Z",
        "end_time": "2025-01-15T05:10:00Z",
        "duration": 600
      },
      {
        "stage": "rem",
        "start_time": "2025-01-15T05:10:00Z",
        "end_time": "2025-01-15T06:30:00Z",
        "duration": 4800
      },
      {
        "stage": "awake",
        "start_time": "2025-01-15T06:30:00Z",
        "end_time": "2025-01-15T07:00:00Z",
        "duration": 1800
      }
    ],
    "today_total_sleep_hours": 7.25
  }
}
```

---

## 数据库存储结构

### 表关系图

```
life_probes (探针表)
    ├─1:N─► life_logger_events (原始事件表)
    ├─1:N─► life_heart_rates (心率表)
    ├─1:N─► life_step_samples (步数样本表)
    ├─1:N─► life_step_daily_totals (每日步数汇总表)
    └─1:N─► life_sleep_segments (睡眠分段表)
```

### 1. life_probes（探针主表）

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `updated_at` | timestamp | - | 更新时间 |
| `deleted_at` | timestamp | - | 软删除时间 |
| `name` | string | - | 探针名称 |
| `device_id` | string | UK | 设备唯一标识符 |
| `description` | text | - | 描述 |
| `tags` | string | - | 标签（逗号分隔） |
| `allow_public_view` | bool | - | 是否允许公开访问 |
| `last_sync_at` | timestamp | - | 最后同步时间 |
| `battery_level` | float | - | 最新电量 (0.0-1.0) |
| `latest_heart_rate` | float | - | 最新心率值 |
| `latest_heart_rate_at` | timestamp | - | 最新心率时间 |

### 2. life_logger_events（原始事件表）

存储所有上传的原始事件，用于审计和调试。

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `life_probe_id` | uint | FK, IDX | 关联的探针 ID |
| `event_id` | string | UK | 事件唯一标识符 |
| `device_id` | string | IDX | 设备 ID（冗余存储） |
| `data_type` | string | IDX | 数据类型 |
| `timestamp` | timestamp | IDX | 事件时间 |
| `battery_level` | float | - | 电量 |
| `payload` | json | - | 原始 JSON 载荷 |

**索引**：
- `event_id`：唯一索引，保证幂等性
- `life_probe_id`：外键索引
- `device_id`, `data_type`, `timestamp`：查询优化

### 3. life_heart_rates（心率数据表）

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `life_probe_id` | uint | FK, UK(probe+time) | 探针 ID |
| `event_id` | string | IDX | 来源事件 ID |
| `measure_time` | timestamp | UK(probe+time) | 测量时间 |
| `value` | float | - | 心率值（BPM） |
| `unit` | string | - | 单位（固定 "bpm"） |

**唯一索引**：`(life_probe_id, measure_time)` - 防止同一时刻重复记录

### 4. life_step_samples（步数样本表）

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `life_probe_id` | uint | FK, UK | 探针 ID |
| `event_id` | string | IDX | 来源事件 ID |
| `sample_type` | string | UK | 样本类型（固定 "steps_detailed"） |
| `start_time` | timestamp | UK, IDX | 区间开始时间 |
| `end_time` | timestamp | UK, IDX | 区间结束时间 |
| `value` | float | - | 步数值 |

**唯一索引**：`(life_probe_id, sample_type, start_time, end_time)` - 防止重复区间

### 5. life_step_daily_totals（每日步数汇总表）

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `life_probe_id` | uint | FK, UK | 探针 ID |
| `day` | date | UK | 日期（UTC 0 点截断） |
| `sample_type` | string | UK | 样本类型 |
| `total` | float | - | 当日累计值 |

**唯一索引**：`(life_probe_id, day, sample_type)` - 每个探针每天一条记录

**聚合逻辑**：
- 自动从 `life_step_samples` 累加
- 跨天区间会按时长比例拆分到不同日期
- 如果上传时提供 `today_total_steps`，会覆盖当天的值

### 6. life_sleep_segments（睡眠分段表）

| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| `id` | uint | PK | 主键 |
| `created_at` | timestamp | - | 创建时间 |
| `life_probe_id` | uint | FK, UK | 探针 ID |
| `event_id` | string | IDX | 来源事件 ID |
| `stage` | string | UK | 睡眠阶段 |
| `start_time` | timestamp | UK | 阶段开始时间 |
| `end_time` | timestamp | UK | 阶段结束时间 |
| `duration` | float | - | 持续时长（秒） |

**唯一索引**：`(life_probe_id, stage, start_time, end_time)` - 防止重复分段

**更新策略**：
- 使用 `ON CONFLICT` 更新：如果同一阶段区间已存在，更新 `event_id` 和 `duration`
- 支持睡眠会话的增量更新（设备可能逐步完善睡眠阶段）

---

## 前端展示逻辑

### 页面结构

1. **探针列表页** (`/admin/life-probes`)
   - 显示所有探针的概览卡片
   - 实时 WebSocket 更新
   - 支持创建、编辑、删除探针

2. **探针详情页** (`/admin/life-probes/{id}`)
   - 多维度数据可视化
   - 时间范围选择：24小时、7天、30天
   - 实时数据推送

### 数据聚合规则

#### 1. 探针概览卡片

显示字段：
- **当前心率**：`life_probes.latest_heart_rate`（直接从探针表读取）
- **今日步数**：
  1. 优先从 `life_step_daily_totals` 查询今日记录
  2. 如果不存在，从 `life_step_samples` 累加今日样本
- **睡眠时长**：
  1. 查询最新的 `life_logger_events`（`data_type = sleep_detailed`）
  2. 获取对应的 `life_sleep_segments`
  3. 累加所有阶段的 `duration`（排除 `in_bed` 阶段）
- **电量**：`life_probes.battery_level`
- **最后同步**：`life_probes.last_sync_at`

#### 2. 心率曲线图

- **数据源**：`life_heart_rates` 表
- **查询条件**：`life_probe_id = ? AND measure_time BETWEEN ? AND ?`
- **排序**：按 `measure_time ASC`
- **前端处理**：
  - X 轴：时间（HH:mm 格式）
  - Y 轴：心率值（BPM）
  - 平滑曲线，面积渐变填充

#### 3. 步数区间柱状图

- **数据源**：`life_step_samples` 表
- **查询条件**：`life_probe_id = ? AND start_time BETWEEN ? AND ?`
- **排序**：按 `start_time ASC`
- **前端处理**：
  - X 轴：区间开始时间（HH:mm）
  - Y 轴：步数值
  - Tooltip 显示完整区间：`start_time - end_time`

#### 4. 每日步数柱状图

- **数据源**：`life_step_daily_totals` 表
- **查询条件**：`life_probe_id = ? AND sample_type = 'steps_detailed' AND day >= ?`
- **排序**：按 `day ASC`
- **前端处理**：
  - X 轴：日期（MM/DD 格式）
  - Y 轴：总步数
  - 显示最近 7 天或 30 天

#### 5. 睡眠阶段分布图

- **数据源**：`life_sleep_segments` 表
- **查询逻辑**：
  1. 查询最新的 `life_logger_events`（`data_type = sleep_detailed`，按 `timestamp DESC` 排序）
  2. 根据 `event_id` 获取所有 `life_sleep_segments`
- **前端处理**：
  - 类型：Custom 自定义图表（横向时间轴分段）
  - X 轴：时间（HH:mm）
  - Y 轴：睡眠阶段（深睡、浅睡、REM、清醒）
  - 每个分段显示为彩色矩形，颜色映射见前文

#### 6. 睡眠质量评级

根据总睡眠时长自动评级：

| 睡眠时长 | 评级 | 颜色 |
|---------|------|------|
| 7-9 小时 | 好 | 绿色 `#52c41a` |
| 6-7 或 9-10 小时 | 中 | 橙色 `#faad14` |
| <6 或 >10 小时 | 差 | 红色 `#ff4d4f` |

### WebSocket 实时推送

#### 1. 探针列表推送

- **连接地址**：`ws://{host}/api/life-probes/public/ws?token={jwt_token}`
- **消息格式**：
  ```json
  {
    "type": "life_probe_list",
    "life_probes": [
      {
        "id": 1,
        "name": "探针名称",
        "device_id": "...",
        "latest_heart_rate": { "time": "...", "value": 72 },
        "steps_today": 5432,
        "sleep_duration": 27000,
        "battery_level": 0.85,
        "last_sync_at": "..."
      }
    ]
  }
  ```
- **推送时机**：
  - 客户端连接时立即推送
  - 每 30 秒定时推送
  - 数据更新时实时推送

#### 2. 探针详情推送

- **连接地址**：`ws://{host}/api/life-probes/public/{id}/ws?token={token}&hours=24&daily_days=7`
- **消息格式**：
  ```json
  {
    "type": "life_probe_detail",
    "life_probe_id": 1,
    "details": {
      "summary": { ... },
      "heart_rates": [ ... ],
      "step_samples": [ ... ],
      "sleep_segments": [ ... ],
      "sleep_overview": { ... }
    }
  }
  ```
- **推送时机**：同探针列表

---

## 第三方适配指南

### 适配步骤

#### 第 1 步：了解系统要求

- 确认支持的数据类型：心率、步数、睡眠
- 确认数据上传接口：`POST /api/life-logger/events`
- 确认时间格式要求：RFC3339（UTC）

#### 第 2 步：注册探针

联系系统管理员创建探针，获取 `device_id`。或使用管理 API：

```bash
POST /api/life-probes
Authorization: Bearer <admin_token>
{
  "name": "第三方设备名称",
  "device_id": "third-party-device-001",
  "description": "来自第三方系统",
  "allow_public_view": false
}
```

#### 第 3 步：数据转换

根据第三方数据源的格式，转换为系统要求的格式：

**示例：从 Apple Health 获取心率**

```python
import datetime
import json
import requests

def upload_heart_rate(device_id, bpm, measure_time):
    event = {
        "event_id": f"hr-{measure_time.isoformat()}-001",
        "device_id": device_id,
        "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "data_type": "heart_rate",
        "payload": {
            "value": bpm,
            "unit": "bpm",
            "measure_time": measure_time.isoformat()
        }
    }

    response = requests.post(
        "http://your-server/api/life-logger/events",
        json=event,
        headers={"Content-Type": "application/json"}
    )

    return response.status_code == 200

# 使用示例
measure_time = datetime.datetime.now(datetime.timezone.utc)
success = upload_heart_rate("third-party-device-001", 72, measure_time)
print(f"上传{'成功' if success else '失败'}")
```

#### 第 4 步：处理幂等性

使用唯一的 `event_id` 保证幂等性：

```python
def generate_event_id(data_type, timestamp, sequence=0):
    """
    生成唯一事件 ID
    格式：{data_type}-{date}-{time}-{sequence}
    """
    dt = timestamp.strftime("%Y%m%d-%H%M%S")
    return f"{data_type}-{dt}-{sequence:03d}"

# 示例
event_id = generate_event_id("heart_rate", datetime.datetime.now(datetime.timezone.utc), 1)
# 输出：heart_rate-20250115-103000-001
```

#### 第 5 步：批量上传

建议策略：

1. **实时上传**（心率）
   - 每次测量后立即上传
   - 适合低频数据（每分钟或更长）

2. **定时批量上传**（步数、睡眠）
   - 每 15 分钟或 1 小时批量上传步数样本
   - 睡眠会话结束后上传完整数据

3. **重试机制**
   - 网络失败时本地缓存
   - 使用相同 `event_id` 重试，保证幂等性

#### 第 6 步：测试验证

1. 使用 ping 接口测试连接：
   ```bash
   POST /api/life-logger/events
   {
     "ping": "test",
     "device_id": "your-device-id",
     "timestamp": "2025-01-15T10:00:00Z"
   }
   ```

2. 上传测试数据并检查前端展示

3. 验证时区处理正确（使用不同时区的测试数据）

---

### 常见问题与解决方案

#### Q1: 上传成功但前端没有数据？

**排查步骤**：

1. 确认 `device_id` 与探针创建时一致
2. 检查时间范围（前端默认显示最近 24 小时）
3. 确认数据类型拼写正确（`heart_rate` / `steps_detailed` / `sleep_detailed`）
4. 检查时间格式是否符合 RFC3339
5. 查看后端日志是否有错误

#### Q2: 数据重复或翻倍？

**原因**：`event_id` 不唯一或重试时未使用相同 `event_id`

**解决**：
- 确保每个事件使用唯一的 `event_id`
- 重试时使用相同的 `event_id`
- 可以在客户端持久化已上传的 `event_id` 列表

#### Q3: 睡眠阶段显示不正确？

**排查步骤**：

1. 检查 `stage` 字段是否使用正确的枚举值（`deep` / `core` / `rem` / `awake` / `in_bed`）
2. 确认时间区间不重叠（或按需处理重叠）
3. 确认 `duration` 与 `end_time - start_time` 一致
4. 检查是否包含 `in_bed` 阶段（会被排除在统计外）

#### Q4: 跨时区问题？

**解决**：
- 所有时间字段必须使用 UTC 时区或明确指定偏移量
- 不要使用本地时间（无时区信息）
- 后端按 UTC 存储，前端按用户时区展示

#### Q5: 步数累计不准确？

**原因**：
- 区间重叠导致重复计算
- 跨天区间未正确处理

**解决**：
- 确保区间不重叠
- 使用 `today_total_steps` 字段覆盖聚合值（推荐）
- 后端会自动处理跨天区间拆分

---

### 适配验收清单

完成以下检查确保适配成功：

- [ ] 探针已创建，`device_id` 已获取
- [ ] 测试 ping 接口返回正确的探针信息
- [ ] 上传心率数据，前端心率曲线正确显示
- [ ] 上传步数数据，前端步数图表正确显示
- [ ] 上传睡眠数据，前端睡眠阶段分布正确显示
- [ ] 所有时间字段使用 UTC 时区
- [ ] `event_id` 唯一且支持幂等重试
- [ ] 睡眠阶段枚举正确映射
- [ ] 跨时区数据展示正确
- [ ] 网络异常时可安全重试
- [ ] 批量上传不超过性能限制（建议每次 < 1000 条样本）

---

## 附录

### A. 错误码参考

| HTTP 状态码 | 错误原因 | 解决方法 |
|------------|---------|---------|
| 400 | 请求体格式错误 | 检查 JSON 格式是否正确 |
| 400 | 缺少必填字段 | 确认 `event_id`, `device_id`, `timestamp`, `data_type`, `payload` 都存在 |
| 400 | `data_type` 不支持 | 使用 `heart_rate` / `steps_detailed` / `sleep_detailed` |
| 400 | `timestamp` 格式错误 | 使用 RFC3339 格式，如 `2025-01-15T10:30:00Z` |
| 400 | Payload 格式错误 | 检查对应数据类型的 payload 结构 |
| 404 | 探针不存在 | 确认 `device_id` 已在系统中注册 |
| 500 | 服务器内部错误 | 联系系统管理员检查日志 |

### B. 性能建议

- **单次上传限制**：建议每次上传 < 1000 条样本，payload < 1MB
- **上传频率**：
  - 心率：每分钟或按测量频率
  - 步数：每 15 分钟至 1 小时
  - 睡眠：会话结束后一次性上传
- **并发限制**：建议单设备最多 5 个并发请求

### C. 数据保留策略

- 默认保留所有历史数据
- 可通过定时任务清理旧数据（管理员配置）
- 建议第三方客户端不依赖长期历史数据查询

### D. 联系支持

如有技术问题，请提供以下信息：

- `device_id`
- 上传时间范围
- 完整的请求体（脱敏后）
- 错误响应内容
- 客户端日志

---

**文档版本**: v1.0
**最后更新**: 2025-12-14
**适用系统版本**: better_monitor v1.2+
