# RushAIO 构建指南（Windows 原生，无 Docker）

本文记录从源码到最终 `RushAIO.exe` 的完整构建过程。最终产物由三块独立组成：

| 产物 | 位置 | 大小 |
|---|---|---|
| Python CLI（主程序） | `cli\RushAIO\RushAIO.exe`（整个 `RushAIO\` 文件夹） | ~5.5MB exe + ~106MB `_internal` |
| Go 后端 | `build\` 下 6 个 exe | 4.8MB ~ 36.5MB |
| Deno geets | `geets\serv.exe`、`geets\serv_ws.exe` | ~80.6MB / ~80.7MB |

> 所有构建都在 `E:\deepseekworkspace` 下完成；源码根目录 `E:\deepseekworkspace\rushaio-master`。

---

## 0. 环境前提：缓存/临时目录必须重定向到 E 盘

本机存在全局限制：**子进程无法写入 `C:\Users\Administrator\go` 和 `C:\Users\Administrator\AppData\Local\Temp`**（Access denied）。所有构建工具必须改用 E 盘目录。

| 项 | 重定向到 |
|---|---|
| `GOPATH` / `GOMODCACHE` / `GOCACHE` / `GOTMPDIR` | `E:\deepseekworkspace\gopath`（子目录 `pkg\mod`、`cache`、`tmp`） |
| `TMP` / `TEMP` | 各自构建临时目录（见下） |
| pip cache | `E:\deepseekworkspace\pybuild\pipcache` |
| Deno cache | `E:\deepseekworkspace\deno_tool\cache` |

---

## 1. Go 工具链

- Go：`go1.27.4.windows-amd64.zip` → 解压到 `E:\deepseekworkspace\gotoolchain\go`
- C 编译器（sqlite3 的 CGO 需要）：MinGW gcc 16.2.0，来自 winget 包 `BrechtSanders.WinLibs.POSIX.UCRT`

```
winget install --id BrechtSanders.WinLibs.POSIX.UCRT --silent --accept-package-agreements
```

其 `mingw64\bin` 路径形如：
`C:\Users\Administrator\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin`

### 构建用环境变量（每个新 shell 都要设）

```powershell
$env:HTTP_PROXY=''; $env:HTTPS_PROXY=''
$env:GOROOT='E:\deepseekworkspace\gotoolchain\go'
$env:GOPATH='E:\deepseekworkspace\gopath'
$env:GOMODCACHE='E:\deepseekworkspace\gopath\pkg\mod'
$env:GOCACHE='E:\deepseekworkspace\gopath\cache'
$env:GOTMPDIR='E:\deepseekworkspace\gopath\tmp'
$env:TMP='E:\deepseekworkspace\gopath\tmp'; $env:TEMP=$env:TMP
$env:GOPROXY='https://goproxy.cn,direct'
$env:PATH='E:\deepseekworkspace\gotoolchain\go\bin;C:\Users\Administrator\AppData\Local\Microsoft\WinGet\Packages\BrechtSanders.WinLibs.POSIX.UCRT_Microsoft.Winget.Source_8wekyb3d8bbwe\mingw64\bin;'+$env:PATH
$env:CGO_ENABLED='1'; $env:CC='x86_64-w64-mingw32-gcc'
```

> `HTTP_PROXY`/`HTTPS_PROXY` 必须清空：本机 7890 代理节点已失效；直连 goproxy.cn 可用。

---

## 2. 拉取 Go 依赖

`src\rush\go.mod`：模块 `rush`，`go 1.14`。三个 `replace` 指向本地 vendored 目录：

- `golang.org/x/net => ../golang.org/x/net` ✅
- `github.com/refraction-networking/utls => ../github.com/refraction-networking/utls` ✅
- `github.com/elazarl/goproxy => ../github.com/elazarl/goproxy` ❌ 目录不存在

**修改 1**：删除 `src\rush\go.mod` 中 elazarl 的 `replace` 行。

```powershell
cd E:\deepseekworkspace\rushaio-master\src\rush
go mod download
```

---

## 3. 编译 Go（6 个 main 包）

不要用 `go build ./...`（会把仅测试用的 `rush/net/internal/socktest` 也编进去，它引用 stdlib 内部包会失败）。逐个编：

```powershell
cd E:\deepseekworkspace\rushaio-master\src\rush
$out='E:\deepseekworkspace\rushaio-master\build'
New-Item -ItemType Directory -Force $out | Out-Null
go build -o $out\task_consumer.exe .\task_consumer
go build -o $out\task_single.exe   .\task_single
go build -o $out\rcpool.exe        .\rcpool
go build -o $out\rclogin.exe       .\rclogin
go build -o $out\ws_test.exe       .\ws\test
go build -o $out\akgen_test.exe    .\akgen\test
```

### 编译期需要修的源码问题（源码 dump 本身有缺口）

1. **`task\fnl.go`**：`lastErr`/`attempts` 声明后未使用（只在 retry 回调里赋值、从不读取）→ 回调签名改为 `func(_ uint, _ error)`。
2. **`task\rt.go:220` + `task\task.go:2450`**：引用 `utls.HelloFirefox_82`，但 vendored utls 只到 `HelloFirefox_65` → 在 `src\github.com\refraction-networking\utls\u_common.go` 加：
   ```go
   HelloFirefox_82 = HelloFirefox_65
   ```
3. **`task\totalwine.go:446` + `task\task.go:3702`**：`TW_STORES_CHLSJ` 常量与 `CheckoutTask.GsCheckout` 方法不存在 → 新建 `src\rush\task\stubs.go`：
   ```go
   package task

   import "fmt"

   const TW_STORES_CHLSJ = ""

   func (t *CheckoutTask) GsCheckout() error {
       return fmt.Errorf("greenspaces.id checkout not implemented")
   }
   ```
   > 注意：这是编译桩，运行时 Total Wine / greenspaces.id 相关功能不可用。
4. **`akgen\test\main.go`**：`samples.Gen(...)` 少传 2 个参数（真实签名 `Gen(url, ua, abck, stage, ajIndx)`）→ 两处调用各补 `"", -1`。
5. **`rcpool` 缺 go.sum**：`golang/protobuf` → `go get github.com/golang/protobuf@v1.4.2`。
   （若误装 v1.3.3 会连锁降级 redis/grpc 等，用 `go get github.com/golang/protobuf@v1.4.2 github.com/go-redis/redis/v8@v8.0.0-beta.10 google.golang.org/grpc@v1.28.0 nhooyr.io/websocket@v1.8.6 github.com/opentracing/opentracing-go@v1.1.0` 恢复。）

---

## 4. Python 依赖（venv）

```powershell
py -3.13 -m venv E:\deepseekworkspace\pybuild\venv
$py='E:\deepseekworkspace\pybuild\venv\Scripts\python.exe'
$env:TMP='E:\deepseekworkspace\pybuild\tmp'; $env:TEMP=$env:TMP
$py -m pip install --cache-dir E:\deepseekworkspace\pybuild\pipcache --upgrade pip setuptools wheel pyinstaller
$py -m pip install --cache-dir E:\deepseekworkspace\pybuild\pipcache cmd2==1.0.1 psutil pyzmq requests sentry-sdk tableformatter appdirs win-unicode-console pyreadline wcwidth
```

### Python 3.13 兼容修复

1. **`cmd2 1.0.1` 需要 `pkg_resources`**（新版 setuptools 已移除）：
   ```powershell
   $py -m pip install --force-reinstall --no-deps setuptools==68.2.2
   ```
2. **`pyreadline 2.1` 用了已移除的 `collections.Callable`** → 编辑
   `E:\deepseekworkspace\pybuild\venv\Lib\site-packages\pyreadline\py3k_compat.py`：
   ```python
   import collections.abc
   # isinstance(x, collections.abc.Callable)
   ```

---

## 5. 修 `cli\rush.py`（3 处）

1. **启动 key 写死**（任意 32 位；原逻辑只校验长度，不校验服务器）：
   ```python
   self.key = '00000000000000000000000000000000'
   ```
2. **`APP_DIR` 兜底**：`appdirs.user_data_dir(...)` 指向 `AppData\Local\RushAIO`（子进程写不了），try/except 失败时改为 exe 同目录 `rush_data\`：
   ```python
   try:
       os.makedirs(os.path.join(APP_DIR, 'chrome'))
   except OSError:
       APP_DIR = os.path.join(os.path.dirname(os.path.abspath(sys.argv[0])), 'rush_data')
       os.makedirs(os.path.join(APP_DIR, 'chrome'), exist_ok=True)
   ```
3. **`kill()` 的 `os.getpid()` 转 str**：
   ```python
   subprocess.call(['taskkill','/F','/T','/PID', str(os.getpid())], ...)
   ```

---

## 6. 准备打包资源（`cli\bin\` 与 `cli\external_real\`）

程序运行时从 `_internal\` 下找这些（路径在 `rush.py` 里写死）：

| 期望路径 | 内容 |
|---|---|
| `_internal\bin\task_consumer-windows-8.1-amd64.exe` | Go `task_consumer.exe` **必须改名** |
| `_internal\bin\rcpool.exe` | Go `rcpool.exe` |
| `_internal\bin\rclogin.exe` | Go `rclogin.exe` |
| `_internal\external\zoneinfo.zip` | 真实 IANA tzdata |

```powershell
$cli='E:\deepseekworkspace\rushaio-master\cli'
$out='E:\deepseekworkspace\rushaio-master\build'
New-Item -ItemType Directory -Force "$cli\bin" | Out-Null
Copy-Item "$out\task_consumer.exe" "$cli\bin\task_consumer-windows-8.1-amd64.exe" -Force
Copy-Item "$out\rcpool.exe"        "$cli\bin\rcpool.exe" -Force
Copy-Item "$out\rclogin.exe"       "$cli\bin\rclogin.exe" -Force
New-Item -ItemType Directory -Force "$cli\external_real" | Out-Null
Copy-Item 'E:\deepseekworkspace\gotoolchain\go\lib\time\zoneinfo.zip' "$cli\external_real\zoneinfo.zip" -Force
```

> 项目自带的 `cli\external` 是 25 字节文本文件（内容是 `../src/rush/task/external`），
> `src\rush\task\external\` 里全是 git-lfs 指针占位（deno/Firefox/chromium/zoneinfo 均无真实文件）。

---

## 7. PyInstaller 打包 —— 必须用 `--onedir`

**不要用 `--onefile`**：单文件 exe 启动前要把自身解压到 `%TEMP%`，本机子进程写不了 `AppData\Local\Temp`，bootloader 会直接报
`[PYI-xxxx:ERROR] Could not create temporary directory!` 后退出（Python 尚未启动，任何日志都写不出来）。

```powershell
cd E:\deepseekworkspace\rushaio-master\cli
$py='E:\deepseekworkspace\pybuild\venv\Scripts\python.exe'
$env:TMP='E:\deepseekworkspace\pybuild\tmp'; $env:TEMP=$env:TMP
& $py -m PyInstaller --onedir --console --name RushAIO --icon rush.ico --paths . `
  --add-data "icon.txt;." --add-data "zyzz.txt;." --add-data "rcfarm/mitm;mitm" `
  --add-data "bin;bin" --add-data "external_real;external" `
  --clean --noconfirm rush.py
```

产物即 `cli\dist\RushAIO\`。最终整理为：

```
cli\RushAIO\
├── RushAIO.exe            5.5MB   ← 双击运行
├── _internal\             ~106MB  （python313.dll + bin\ + external\ + mitm\ 等）
└── rush_data\                      （运行时生成，存 rush.db / key）
```

`cli\START_RushAIO.cmd` 是一个简单启动器（切到目录再启动）。

---

## 8. Deno geets

```powershell
$env:DENO_DIR='E:\deepseekworkspace\deno_tool\cache'
$env:TMP='E:\deepseekworkspace\deno_tool\tmp'; $env:TEMP=$env:TMP
$deno='E:\deepseekworkspace\deno_tool\deno.exe'
cd E:\deepseekworkspace\rushaio-master\geets
& $deno compile --allow-all --output serv.exe    serv.http.js
& $deno compile --allow-all --output serv_ws.exe serv.ws.js
```

> `deno check` 在 Deno 2.9.8 + TS 6.0.3 下对旧 std 会类型报错，直接 `deno compile` 即可（不强制类型检查）。

---

## 9. 运行时的拼装关系

```
用户双击 RushAIO.exe
   ├─ Python CLI（cmd2 界面）
   │    ├─ 读 _internal\icon.txt / zyzz.txt
   │    ├─ APP_DIR = RushAIO\rush_data（rush.db、key）
   │    ├─ zmq PUB 42423 / SUB_TAIL 42425
   │    └─ spawn _internal\bin\task_consumer-windows-8.1-amd64.exe（Go 后端）
   │         └─ 依赖 _internal\external\zoneinfo.zip（time.LoadLocation）
   ├─ geets\serv.exe（独立，1996 端口）
   └─ api_py\server.py（独立 Flask，可选）
```

---

## 10. 验证清单

```powershell
# 管道模式冒烟测试
$env:TMP='C:\Users\Administrator\AppData\Local\Temp'; $env:TEMP=$env:TMP
'quit' | & 'E:\deepseekworkspace\rushaio-master\cli\RushAIO\RushAIO.exe' 2>&1 | Select-Object -First 5
# 应看到 banner + "RushAIO 0.2.44" + "Type 'help' to explore commands"
```

独立控制台验证（模拟双击）可用 `subprocess.Popen([exe], creationflags=subprocess.CREATE_NEW_CONSOLE)`，几秒后仍存活即说明正常等待输入。

---

## 11. 已知缺口（源码 dump 自带，非构建失败）

1. `cli\external\` 里的 `deno.exe`、`Firefox`、`chromium` 等均为 **git-lfs 指针占位**，真实文件未随包提供 → 浏览器自动化功能需另补资源（`zoneinfo.zip` 已用 Go 工具链的真文件替代）。
2. `geets\serv.http.js` 运行时 spawn `worker.bundle.js`，但仓库只有 `worker.js`（原靠 `deno bundle` 生成，Deno 2.x 已移除）→ 需另行打 bundle。
3. `api_py\` 是 Flask 服务端（非 exe 目标），依赖为 2020 年代旧锁（`Flask==1.1.2`、`PyJWT==1.7.1`、`pycryptodome==3.9.8`），在 Python 3.13 上尚未验证。

---

## 附：产物清单

```
E:\deepseekworkspace\rushaio-master\
├── build\                       Go 6 个 exe
│   ├── task_consumer.exe        31.5MB
│   ├── task_single.exe          36.5MB
│   ├── rcpool.exe               26.5MB
│   ├── rclogin.exe              16.8MB
│   ├── ws_test.exe              23.0MB
│   └── akgen_test.exe            4.8MB
├── cli\RushAIO\                 Python CLI（双击 RushAIO.exe）
│   ├── RushAIO.exe
│   └── _internal\
├── cli\START_RushAIO.cmd        启动器
└── geets\
    ├── serv.exe                 80.6MB
    └── serv_ws.exe              80.7MB
```
