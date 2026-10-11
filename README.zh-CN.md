# debounce

适用于 Windows、Linux 和 macOS 的跨进程**尾沿防抖**命令行工具，基于
[oalders/debounce](https://github.com/oalders/debounce)，由
[liang-chang](https://github.com/liang-chang) 独立维护。本版本将上游的“成功执行后的冷却期”
改为“等待一段时间没有新触发后执行最新命令”。macOS 支持为实验性。

[English](README.md)

## 安装

从 [Releases](https://github.com/liang-chang/debounce/releases) 下载对应系统和架构的压缩包，
解压后将可执行文件所在目录加入 `PATH`。Windows 使用 ZIP，Linux/macOS 使用 tar.gz，
同时提供 Linux DEB/RPM 安装包。

使用 Go 1.24 或更新版本安装：

```text
go install github.com/liang-chang/debounce@latest
debounce --version
```

首个独立维护版本为 `0.6.0`（标签 `v0.6.0`）。问题反馈请前往
[liang-chang/debounce/issues](https://github.com/liang-chang/debounce/issues)。

## 编译

需要 Go 1.24 或更新版本。不需要 CGO、外部文件锁工具或安装后台服务。

```powershell
go build -o debounce.exe .
.\debounce.exe --key repo-check 3 s git status --short
```

Linux/macOS：`go build -o debounce .`。

上游发布包**不包含**本实现。原许可证文件和上游版本历史均予以保留，以说明来源。

## 执行流程

1. 首次调用成为协调者并保持前台运行，等待完整的静默窗口后才执行命令。
2. 相同任务标识的后续调用提交最新命令并重置窗口。收到确认后输出
   `Existing coordinator: waiting; trigger accepted`（或 `running`），并以退出码 0 结束。
3. 静默窗口结束后，协调者执行**最后接受的命令**。顺序以协调者接受请求的先后为准，
   而不是以客户端进程启动时间为准。
4. 执行期间的新触发不会中断当前命令，而是合并为下一轮待执行命令。
   当前命令结束且下一轮静默窗口到期后，才开始下一轮执行。
5. 没有正在执行或待执行的命令时，协调者关闭 IPC 端点并退出。
   再次调用会开启新的静默窗口。

例如窗口为 3 秒，在第 0、1、2 秒触发，最终约在第 5 秒执行一次。
执行结束后**没有成功冷却期**。

## 用法与任务标识

```text
debounce [flags] <integer> <unit> <command> [arguments...]
```

时间单位：`s/second/seconds`、`m/minute/minutes`、`h/hour/hours`、`d/day/days`。
一天按 24 小时计算，数量必须在 1 到 36500 之间。选项放在时间数量之前，
可用 `--` 分隔目标命令。

- `--key KEY`：显式指定逻辑任务标识。相同 key 共用一个协调者，即使命令或工作目录不同。
- 不指定 `--key`：根据带版本的命令及参数结构生成标识，保留参数边界和原始 Unix 字节。
- `--local`：将清理并解析符号链接后的工作目录加入自动生成的标识；不影响显式 key。
- `--status`：查询 `waiting`、`running` 或 `No active coordinator`，不提交触发、
  不改变计时器。查询时需提供相同的任务标识。
- `--debug`：输出 IPC 端点和协调者状态变化。
- `--version`、`--help`：显示版本和用法。

所有标识均限定在当前用户的 UID/SID 范围内。不提供 `--namespace`。
`--state-dir` 和上游的 `--cache-dir` 已移除。

同一任务标识允许使用不同间隔，最后接受的触发决定下一条命令及其静默窗口长度。

## 命令执行与结果

命令直接执行，不会隐式经过 shell。CMD 内置命令需使用 `cmd.exe /c`，
PowerShell 脚本可使用 `pwsh.exe -NoProfile -File job.ps1`。
提交进程解析可执行文件，并将其绝对路径、参数和工作目录发送给协调者。

**不传递环境变量**。所有子命令继承协调者的环境，即首次调用时的环境。
后续调用者的环境变化不会生效。

目标命令的输出会被缓冲，在协调者退出时打印到**协调者所在的终端**。
提交客户端收到的是请求确认，而非目标命令的最终结果。
任意命令或 IPC 服务操作失败时，协调者以退出码 1 结束，否则为 0。
命令失败不会丢弃已接受的下一轮触发，也不会自动重试该命令。

正常关闭期间收到的请求会得到重试响应，调用者重新连接或竞争成为下一任协调者。
如果请求可能已送达但确认失败，客户端会报告**结果不确定**并以非零状态退出，
不会自动重放可能已被接受的命令。

## 平台支持与边界

- **Linux：** 使用抽象 Unix 域流式 socket，不创建 `.sock` 文件。
  所有 socket 引用关闭后由内核移除端点，检查对端凭据是否属于当前 UID。
  协调范围限定在同一个 Linux 网络命名空间内。
- **Windows：** 通过 `github.com/Microsoft/go-winio` 使用独占命名管道。
  ACL 仅允许当前用户的 SID。用户身份直接从进程令牌获取，不要求账户存在用户配置目录。
  客户端发送命令数据前还会检查管道服务端进程令牌的 SID。
- **macOS（实验性）：** 在当前用户专用、权限为 `0700` 的
  `/tmp/debounce-v2-<uid>` 目录中使用路径 Unix 域 socket，并校验对端 UID。
  协调者在整个生命周期内持有该任务的 `flock`。只有持锁者可清理崩溃后的残留 socket。
  正常关闭时先删除 socket 再释放锁，空 `.lock` 文件保留以确保锁对应同一个文件。
  程序仍在运行时不要删除该目录或锁文件。
- 其他系统会返回明确的不支持平台错误。

Linux/Windows 不持久保存锁、时间戳或 socket 文件。macOS 保留空锁文件，
崩溃后也可能残留 socket 文件。请求仅保存在内存中。
崩溃、强制终止或重启可能导致待执行任务丢失。目标命令可能在协调者退出后继续运行；
本版本不保证孤儿进程互斥、自动恢复任务或跨崩溃的恰好一次执行。

首次调用者必须保持存活直到工作完成。本工具不是脱离终端的调度服务、分布式锁或文件监视器。
独立仓库或任务应使用不同 key。所有参与协调的调用应使用相同实现和协议。

单个触发请求上限为 1 MiB。IPC 交互超时为 5 秒，启动和关闭过程中的连接竞争最多重试 5 秒。

## 验证与许可证

```text
go test ./...
go vet ./...
```

集成测试启动独立进程，覆盖计时重置、最新命令选择、排队执行、环境与工作目录、
独立 key、失败处理、自动退出及并发选主。Linux CI 还使用竞态检测。
交叉编译不能替代 IPC 运行测试。

保留上游 Apache-2.0/MIT 许可证文件。第三方 IPC 依赖及其许可证见
[`THIRD-PARTY-NOTICES`](THIRD-PARTY-NOTICES)。

## 平台测试说明

本项目主要在 **Windows** 上使用和验证。

- **Windows：** 本地执行 Go 测试和静态检查。
- **Linux：** 使用 WSL2 Debian 执行 Linux 测试程序，验证防抖、并发协调和
  命令执行；WSL 结果不代表已覆盖独立 Linux 主机的所有环境。
- **macOS：** 维护者暂无 Mac 机器，**未做本地真机运行测试**。本地仅进行交叉编译，
  并在 WSL 中测试共用的路径 socket/文件锁逻辑；这不能替代 macOS 专属
  身份校验及实际运行验证，因此 macOS 支持暂标为实验性。

GitHub Actions 已在 Windows、Linux 和 macOS 上运行测试并通过；macOS 仍缺少
维护者本地真机使用验证，因此继续标为实验性支持。
