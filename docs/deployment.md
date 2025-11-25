# Better Monitor 部署文档

## 概述

Better Monitor 支持多种部署方式，包括 Docker 容器化部署、源码编译部署和一键自动化部署。本文档详细说明各种部署方式的具体步骤和注意事项。

## 系统要求

### 硬件要求
- **CPU**: 1 核心以上
- **内存**: 1GB 以上
- **磁盘**: 10GB 以上可用空间
- **网络**: 支持 HTTP/HTTPS 访问

### 软件要求
- **操作系统**: Linux/Windows/macOS
- **Docker**: 20.10+ (容器化部署)
- **Docker Compose**: 2.0+ (推荐)
- **Go**: 1.21+ (源码编译)
- **Node.js**: 18+ (前端编译)

### 端口要求
- **3333**: Dashboard 主服务
- **80/443**: Nginx 代理（可选）

## 部署方式

### 方式一：Docker 容器化部署（推荐）

#### 1. 快速部署

使用预构建的 Docker 镜像进行快速部署：

```bash
# 创建项目目录
mkdir better-monitor && cd better-monitor

# 创建 docker-compose.yml
cat > docker-compose.yml << 'EOF'
version: '3.8'

services:
  better-monitor:
    image: enderhkc/better-monitor:latest
    container_name: better-monitor
    restart: unless-stopped
    ports:
      - "3333:3333"
    volumes:
      - ./data:/app/data:rw
      - ./logs:/app/logs:rw
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - TZ=Asia/Shanghai
      - JWT_SECRET=your_jwt_secret_key_change_this_in_production
      - VERSION=1.0.5
    security_opt:
      - no-new-privileges:true
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3333/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s

EOF

# 启动服务
docker-compose up -d

# 查看运行状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

#### 2. 源码构建部署

从源码构建 Docker 镜像：

```bash
# 克隆源码
git clone https://github.com/your-repo/better-monitor.git
cd better-monitor

# 构建并启动
docker-compose -f docker-compose.all-in-one.yml up -d --build

# 查看构建日志
docker-compose -f docker-compose.all-in-one.yml logs -f better-monitor
```

#### 3. 生产环境部署

生产环境建议使用以下配置：

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  better-monitor:
    image: enderhkc/better-monitor:latest
    container_name: better-monitor
    restart: unless-stopped
    ports:
      - "3333:3333"
    volumes:
      - ./data:/app/data:rw
      - ./logs:/app/logs:rw
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./ssl:/app/ssl:ro
    environment:
      - TZ=Asia/Shanghai
      - JWT_SECRET=${JWT_SECRET}
      - VERSION=${VERSION:-1.0.5}
      - DB_TYPE=mysql
      - DB_DSN=${DB_DSN}
      - REDIS_URL=${REDIS_URL}
      - SMTP_HOST=${SMTP_HOST}
      - SMTP_PORT=${SMTP_PORT}
      - SMTP_USER=${SMTP_USER}
      - SMTP_PASS=${SMTP_PASS}
    security_opt:
      - no-new-privileges:true
    read_only: false
    tmpfs:
      - /tmp:rw,noexec,nosuid,size=100m
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3333/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
    depends_on:
      - mysql
      - redis

  mysql:
    image: mysql:8.0
    container_name: better-monitor-mysql
    restart: unless-stopped
    environment:
      - MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}
      - MYSQL_DATABASE=better_monitor
      - MYSQL_USER=better_monitor
      - MYSQL_PASSWORD=${MYSQL_PASSWORD}
    volumes:
      - ./mysql-data:/var/lib/mysql
    ports:
      - "3306:3306"
    command: --default-authentication-plugin=mysql_native_password

  redis:
    image: redis:7-alpine
    container_name: better-monitor-redis
    restart: unless-stopped
    volumes:
      - ./redis-data:/data
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes

  nginx:
    image: nginx:alpine
    container_name: better-monitor-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    depends_on:
      - better-monitor
```

创建环境变量文件：

```bash
# .env
JWT_SECRET=your_very_secure_jwt_secret_key_here
VERSION=1.0.5
DB_DSN=better_monitor:password@tcp(mysql:3306)/better_monitor?charset=utf8mb4&parseTime=True&loc=Local
REDIS_URL=redis://redis:6379
MYSQL_ROOT_PASSWORD=your_mysql_root_password
MYSQL_PASSWORD=your_mysql_password
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your_email@gmail.com
SMTP_PASS=your_email_password
```

### 方式二：二进制包部署

Better Monitor 现在通过 GitHub Releases 分发所有组件的构建产物。对于不使用 Docker 的场景，可以按以下步骤部署：

1. 访问 [Releases](https://github.com/your-repo/better-monitor/releases) 下载对应平台的后端、前端与 Agent 压缩包。
2. 将后端二进制解压到 `/opt/better-monitor/backend`，并准备 `.env` 或 `config.yaml`。
3. 使用 `systemd` 创建服务：
   ```bash
   sudo tee /etc/systemd/system/better-monitor.service <<'EOF'
   [Unit]
   Description=Better Monitor Dashboard
   After=network.target

   [Service]
   WorkingDirectory=/opt/better-monitor/backend
   ExecStart=/opt/better-monitor/backend/better-monitor-backend
   Restart=on-failure
   Environment=JWT_SECRET=change_me

   [Install]
   WantedBy=multi-user.target
   EOF
   sudo systemctl enable --now better-monitor
   ```
4. 前端静态资源可以放在 Nginx/Apache 的 `root` 目录或 CDN。
5. Agent 通过 `server_id`、`secret_key` 与 `better-monitor-agent` 结合 `systemd`/`Task Scheduler` 部署，流程与 README 中一致。

### 方式三：源码编译部署

#### 1. 前端编译

```bash
# 进入前端目录
cd frontend

# 安装依赖
npm install

# 构建生产版本
npm run build

# 构建结果位于 dist/ 目录
ls -la dist/
```

#### 2. 后端编译

```bash
# 进入后端目录
cd backend

# 下载依赖
go mod tidy

# 构建二进制文件
go build -ldflags="-w -s -X main.version=1.0.5 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o better-monitor-backend main.go

# 运行
./better-monitor-backend
```

#### 3. Agent 编译

```bash
# 进入 Agent 目录
cd agent

# 下载依赖
go mod tidy

# 构建二进制文件
go build -ldflags="-w -s -X main.version=1.0.5 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o better-monitor-agent cmd/agent/main.go

# 运行
./better-monitor-agent -config config/agent.yaml
```

#### 4. 批量构建

```bash
# 使用批量构建脚本
cd Releases
python build.py --all

# 构建特定平台
python build.py --platforms linux,windows,darwin --version 1.0.5
```

## 高级配置

### 1. 反向代理配置

#### Nginx 配置

```nginx
# /etc/nginx/sites-available/better-monitor
upstream better_monitor {
    server 127.0.0.1:3333;
    keepalive 32;
}

upstream better_monitor_ota {
    server 127.0.0.1:8086;
    keepalive 32;
}

server {
    listen 80;
    server_name your-domain.com;
    
    # 强制跳转到 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    # SSL 证书配置
    ssl_certificate /path/to/your/cert.pem;
    ssl_certificate_key /path/to/your/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    
    # 安全标头
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;
    
    # 日志配置
    access_log /var/log/nginx/better-monitor.access.log;
    error_log /var/log/nginx/better-monitor.error.log;
    
    # 主应用代理
    location / {
        proxy_pass http://better_monitor;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        proxy_buffering off;
    }
    
    # WebSocket 代理
    location /ws {
        proxy_pass http://better_monitor;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }
    
    # 静态文件缓存
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        proxy_pass http://better_monitor;
        proxy_set_header Host $host;
        expires 1y;
        add_header Cache-Control "public, immutable";
        add_header X-Cache-Status $upstream_cache_status;
    }
}
```

#### Apache 配置

```apache
# /etc/apache2/sites-available/better-monitor.conf
<VirtualHost *:80>
    ServerName your-domain.com
    Redirect permanent / https://your-domain.com/
</VirtualHost>

<VirtualHost *:443>
    ServerName your-domain.com
    
    # SSL 配置
    SSLEngine on
    SSLCertificateFile /path/to/your/cert.pem
    SSLCertificateKeyFile /path/to/your/key.pem
    SSLProtocol TLSv1.2 TLSv1.3
    
    # 安全标头
    Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
    Header always set X-Frame-Options DENY
    Header always set X-Content-Type-Options nosniff
    Header always set X-XSS-Protection "1; mode=block"
    
    # 主应用代理
    ProxyPreserveHost On
    ProxyRequests Off
    ProxyPass / http://127.0.0.1:3333/
    ProxyPassReverse / http://127.0.0.1:3333/
    
    # WebSocket 代理
    ProxyPass /ws ws://127.0.0.1:3333/ws
    ProxyPassReverse /ws ws://127.0.0.1:3333/ws
    
    # 日志配置
    CustomLog /var/log/apache2/better-monitor.access.log combined
    ErrorLog /var/log/apache2/better-monitor.error.log
</VirtualHost>
```

### 2. 数据库配置

#### MySQL 配置

```sql
-- 创建数据库
CREATE DATABASE better_monitor CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 创建用户
CREATE USER 'better_monitor'@'%' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON better_monitor.* TO 'better_monitor'@'%';
FLUSH PRIVILEGES;
```

#### PostgreSQL 配置

```sql
-- 创建数据库
CREATE DATABASE better_monitor;

-- 创建用户
CREATE USER better_monitor WITH ENCRYPTED PASSWORD 'your_password';
GRANT ALL PRIVILEGES ON DATABASE better_monitor TO better_monitor;
```

### 3. 系统服务配置

#### systemd 服务配置

```ini
# /etc/systemd/system/better-monitor.service
[Unit]
Description=Better Monitor Server
After=network.target mysql.service

[Service]
Type=simple
User=better-monitor
Group=better-monitor
WorkingDirectory=/opt/better-monitor
ExecStart=/opt/better-monitor/better-monitor-backend
Environment=JWT_SECRET=your_jwt_secret
Environment=DB_DSN=better_monitor:password@tcp(localhost:3306)/better_monitor?charset=utf8mb4&parseTime=True&loc=Local
Restart=always
RestartSec=10
KillMode=mixed
KillSignal=SIGTERM

[Install]
WantedBy=multi-user.target
```

```bash
# 启用并启动服务
sudo systemctl daemon-reload
sudo systemctl enable better-monitor
sudo systemctl start better-monitor

# 查看状态
sudo systemctl status better-monitor
```

## 安全配置

### 1. 防火墙配置

```bash
# UFW 配置
sudo ufw allow 22/tcp
sudo ufw allow 3333/tcp
sudo ufw allow 8086/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable

# iptables 配置
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 3333 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8086 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT
sudo iptables -P INPUT DROP
sudo iptables-save > /etc/iptables/rules.v4
```

### 2. SSL 证书配置

#### Let's Encrypt 证书

```bash
# 安装 certbot
sudo apt-get install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期
sudo crontab -e
# 添加：0 12 * * * /usr/bin/certbot renew --quiet
```

#### 自签名证书

```bash
# 生成私钥
openssl genrsa -out private.key 2048

# 生成证书请求
openssl req -new -key private.key -out certificate.csr

# 生成自签名证书
openssl x509 -req -in certificate.csr -signkey private.key -out certificate.crt -days 365
```

### 3. 数据备份

#### 数据库备份

```bash
#!/bin/bash
# backup.sh
BACKUP_DIR="/backup/better-monitor"
DATE=$(date +%Y%m%d_%H%M%S)

# 创建备份目录
mkdir -p $BACKUP_DIR

# 备份 MySQL
mysqldump -u better_monitor -p better_monitor > $BACKUP_DIR/db_backup_$DATE.sql

# 备份数据文件
tar -czf $BACKUP_DIR/data_backup_$DATE.tar.gz /opt/better-monitor/data

# 清理旧备份（保留7天）
find $BACKUP_DIR -name "*.sql" -mtime +7 -delete
find $BACKUP_DIR -name "*.tar.gz" -mtime +7 -delete
```

#### 配置定时备份

```bash
# 添加到 crontab
sudo crontab -e
# 添加：0 2 * * * /opt/better-monitor/backup.sh
```

## 监控和维护

### 1. 日志管理

```bash
# 查看应用日志
docker-compose logs -f better-monitor

# 查看系统日志
sudo journalctl -u better-monitor -f

# 日志轮转配置
sudo tee /etc/logrotate.d/better-monitor << 'EOF'
/var/log/better-monitor/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 better-monitor better-monitor
    postrotate
        systemctl reload better-monitor
    endscript
}
EOF
```

### 2. 性能监控

```bash
# 监控脚本
#!/bin/bash
# monitor.sh
while true; do
    # 检查内存使用
    MEMORY=$(free -m | grep Mem | awk '{print $3/$2 * 100.0}')
    echo "Memory usage: $MEMORY%"
    
    # 检查磁盘使用
    DISK=$(df -h | grep /dev/sda1 | awk '{print $5}' | sed 's/%//')
    echo "Disk usage: $DISK%"
    
    # 检查服务状态
    STATUS=$(systemctl is-active better-monitor)
    echo "Service status: $STATUS"
    
    sleep 60
done
```

### 3. 故障处理

#### 常见问题解决

```bash
# 1. 服务无法启动
sudo systemctl status better-monitor
sudo journalctl -u better-monitor --since "1 hour ago"

# 2. 端口被占用
sudo lsof -i :3333
sudo netstat -tlnp | grep 3333

# 3. 数据库连接失败
mysql -u better_monitor -p -h localhost better_monitor

# 4. 内存不足
free -m
sudo swapon --show
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# 5. 磁盘空间不足
df -h
sudo du -sh /var/log/* | sort -rh | head -10
sudo find /var/log -name "*.log" -type f -size +100M -exec rm -f {} \;
```

## 升级和更新

### 1. Docker 升级

```bash
# 拉取最新镜像
docker-compose pull

# 重启服务
docker-compose up -d

# 清理旧镜像
docker image prune -a
```

### 2. 源码升级

```bash
# 备份当前版本
cp -r /opt/better-monitor /opt/better-monitor.backup

# 获取新版本
git pull origin main

# 重新构建
docker-compose -f docker-compose.all-in-one.yml up -d --build
```

### 3. 数据库迁移

```bash
# 运行迁移脚本
./migrate.sh

# 验证数据完整性
./verify.sh
```

## 部署检查清单

### 部署前检查

- [ ] 确认系统要求满足
- [ ] 准备域名和 SSL 证书
- [ ] 配置防火墙和安全组
- [ ] 准备数据库和存储
- [ ] 设置监控和告警

### 部署后验证

- [ ] 访问 Web 界面正常
- [ ] 用户登录功能正常
- [ ] Agent 连接正常
- [ ] WebSocket 连接正常
- [ ] 文件上传下载正常
- [ ] 终端功能正常
- [ ] Docker 管理正常
- [ ] 告警通知正常

### 性能优化

- [ ] 启用 Gzip 压缩
- [ ] 配置静态文件缓存
- [ ] 数据库索引优化
- [ ] 连接池配置优化
- [ ] 日志轮转配置

## 故障排查

### 常见错误及解决方案

1. **容器启动失败**
   - 检查端口占用
   - 查看容器日志
   - 验证配置文件

2. **数据库连接失败**
   - 检查数据库服务状态
   - 验证连接字符串
   - 检查用户权限

3. **Agent 连接失败**
   - 检查网络连通性
   - 验证 Secret Key
   - 查看 Agent 日志

4. **WebSocket 连接失败**
   - 检查代理配置
   - 验证 JWT Token
   - 查看浏览器控制台

## 技术支持

如遇到部署问题，请：

1. 查看 [故障排查指南](../README.md#故障排查)
2. 检查 [常见问题](../README.md#常见问题)
3. 提交 [Issue](https://github.com/your-repo/better-monitor/issues)
4. 联系技术支持：support@better-monitor.com

---

**注意**: 生产环境部署时，请务必修改默认密码、密钥等敏感信息，并定期进行安全更新。
