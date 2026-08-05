打包编译：
一、前端：
cd frontend
pnpm build

二、后端：
cd backend
go build -tags=embed -ldflags="-s -w" -trimpath -o bin/server ./cmd/server



# 在 Linux 服务器上
sudo useradd -r -s /bin/false sub2api
sudo mkdir -p /opt/sub2api /etc/sub2api
sudo cp server-linux-amd64 /opt/sub2api/sub2api
sudo chmod +x /opt/sub2api/sub2api
sudo chown -R sub2api:sub2api /opt/sub2api


# PowerShell
cd backend
Set-Location "F:\other\sub2api\backend"
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -tags=embed -ldflags="-s -w" -trimpath -o "bin/server-linux-amd64" ./cmd/server


# 在服务器验证文件
file server-linux-amd64
# 找路径
systemctl show -p ExecStart --value sub2api


shell

# 1. 确认新文件确实是 Linux amd64 程序
file ./server-linux-amd64

# 2. 停止服务
systemctl stop sub2api

# 3. 备份当前运行程序
cp -a ./sub2api "./sub2api.bak.$(date +%Y%m%d-%H%M%S)"

# 4. 安装新程序，并一次性设置正确用户、用户组和权限
install -o sub2api -g sub2api -m 0755 ./server-linux-amd64 ./sub2api

# 5. 检查
ls -lh ./sub2api

# 6. 启动服务
systemctl start sub2api
