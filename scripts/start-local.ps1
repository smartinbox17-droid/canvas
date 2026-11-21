$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$FrontendPort = if ($env:FRONTEND_PORT) { $env:FRONTEND_PORT } else { "3003" }
$BackendPort = if ($env:BACKEND_PORT) { $env:BACKEND_PORT } else { "8080" }
$AgentPort = if ($env:AGENT_PORT) { $env:AGENT_PORT } else { "8090" }

foreach ($cmd in @("go", "node", "npm")) {
  if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
    Write-Error "缺少依赖命令: $cmd，请先安装 Go 1.25+ 与 Node.js 20+（或改用 docker compose -f docker-compose.deploy.yml up -d --build）。"
    exit 1
  }
}

Write-Host "[1/3] 启动 yingce-agent Sidecar (:$AgentPort)..."
$env:YINGCE_AGENT_PORT = $AgentPort
$agentProc = Start-Process -FilePath "node" -ArgumentList "server.mjs" -WorkingDirectory "$RootDir\yingce-agent" -PassThru

Write-Host "[2/3] 编译并启动 Go 后端 (:$BackendPort)..."
Push-Location "$RootDir\backend"
go build -o yingce-backend.exe .\cmd\server
Pop-Location

$env:CANVAS_BACKEND_ADDR = ":$BackendPort"
$env:CANVAS_DATA_PATH = "$RootDir\data"
$env:CANVAS_DATABASE_DRIVER = "sqlite"
$env:CANVAS_AUTO_MIGRATE = "true"
$env:CANVAS_REGISTRATION_ENABLED = "true"
$env:CANVAS_ALLOW_PRIVATE_UPSTREAMS = "true"
$env:CANVAS_CORS_ORIGINS = "http://localhost:$FrontendPort,http://127.0.0.1:$FrontendPort"
$env:YINGCE_AGENT_URL = "http://127.0.0.1:$AgentPort"
$backendProc = Start-Process -FilePath "$RootDir\backend\yingce-backend.exe" -WorkingDirectory "$RootDir\backend" -PassThru

Write-Host "[3/3] 启动 React 前端工作台 (:$FrontendPort)..."
if (-not (Test-Path "$RootDir\web\node_modules")) {
  Push-Location "$RootDir\web"
  npm install
  Pop-Location
}
$env:VITE_API_PROXY_TARGET = "http://127.0.0.1:$BackendPort"
$webProc = Start-Process -FilePath "npm.cmd" -ArgumentList "run","dev","--","--port",$FrontendPort,"--host","0.0.0.0" -WorkingDirectory "$RootDir\web" -PassThru

for ($i = 0; $i -lt 30; $i++) {
  try {
    Invoke-RestMethod -Uri "http://127.0.0.1:$BackendPort/api/health/ready" -TimeoutSec 2 | Out-Null
    try {
      Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$BackendPort/api/v1/auth/register" `
        -ContentType "application/json" `
        -Body '{"email":"admin@canvas.local","password":"admin123456"}' -TimeoutSec 2 | Out-Null
    } catch {}
    break
  } catch {
    Start-Sleep -Seconds 1
  }
}

Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host "  AI Canvas Workbench 本地部署已就绪！" -ForegroundColor Green
Write-Host "  访问地址: http://localhost:$FrontendPort" -ForegroundColor Cyan
Write-Host "  演示管理员: admin@canvas.local / admin123456 (100 初始积分)" -ForegroundColor Yellow
Write-Host "  按 Enter 键停止全部服务..." -ForegroundColor Gray
Write-Host "============================================================" -ForegroundColor Green
Read-Host

foreach ($p in @($webProc, $backendProc, $agentProc)) {
  if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
}
