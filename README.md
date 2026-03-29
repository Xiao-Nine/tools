# tools

一个用 Go 编写的个人命令行工具集合，目前包含三个子命令：

- `ls`：简化版目录查看工具
- `inject`：将本机 SSH 公钥注入远程服务器，并自动追加本地 `~/.ssh/config`
- `sysinfo`：显示当前系统信息（CPU、内存、磁盘、Docker）

项目基于 [Cobra](https://github.com/spf13/cobra) 构建命令行接口。

## 当前功能

### 1. `tools ls`

列出目录内容，默认查看当前目录。

支持能力：

- 默认隐藏以 `.` 开头的文件
- 支持 `-a/--all` 显示隐藏文件
- 支持 `-l/--long` 以长格式输出
- 目录名称会追加 `/`
- 文件名按不区分大小写排序

### 2. `tools sysinfo`

显示当前系统的概览信息，以彩色终端 UI 输出（Windows 下自动禁用颜色）。

显示内容：

- **System**：主机名、操作系统、内核版本、运行时长、进程数
- **CPU**：型号、核心数、实时使用率（带进度条）
- **Memory**：已用 / 总量、使用率进度条、可用量
- **Disk**：各挂载点已用 / 总量及使用率进度条（macOS 仅显示 `/` 和 `/Volumes/*`）
- **Docker**：运行中的容器列表（Docker 不可用时显示提示）

```bash
tools sysinfo
```

### 3. `tools inject`

将本机公钥写入远程服务器的 `~/.ssh/authorized_keys`，方便后续使用密钥登录。

支持两种模式：

- 单台服务器注入：通过 `--server`
- 批量服务器注入：通过 `--servers` 指定 YAML 配置文件

执行完成后，还会尝试把目标机器写入本地 `~/.ssh/config`。

## 环境要求

- Go `1.24.1` 或更高版本

## 构建与运行

在项目根目录执行：

```bash
go build -o bin/tools .
```

直接运行：

```bash
./bin/tools --help
```

或者不构建，直接使用：

```bash
go run . --help
```

## 命令说明

### `tools`

查看根命令帮助：

```bash
tools --help
```

当前可用子命令：

- `ls`
- `inject`
- `completion`

### `tools ls`

基本用法：

```bash
tools ls [path]
```

示例：

```bash
tools ls
tools ls /tmp
tools ls -a
tools ls -l
tools ls -al /var/log
```

参数：

- `-a, --all`：显示隐藏文件
- `-l, --long`：长格式显示

### `tools inject`

基本用法：

```bash
tools inject [flags]
```

参数：

- `--pubkey`：指定公钥文件路径，默认使用 `~/.ssh/id_rsa.pub`
- `--server`：指定单台服务器连接信息，格式为 `username:password@ip:port`
- `--servers`：指定批量服务器 YAML 配置文件

#### 单台服务器示例

```bash
tools inject --server root:123456@192.168.1.10:22
```

如果不写端口，默认使用 `22`：

```bash
tools inject --server root:123456@192.168.1.10
```

指定自定义公钥：

```bash
tools inject --pubkey ~/.ssh/id_ed25519.pub --server root:123456@192.168.1.10:22
```

#### 批量服务器示例

```bash
tools inject --servers config/servers.yaml
```

YAML 配置格式如下：

```yaml
servers:
  - host: "192.168.1.10"
    port: "22"
    username: "root"
    password: "123456"
  - host: "192.168.1.11"
    port: "22"
    username: "deploy"
    password: "password"
```

## `inject` 的实际行为

执行 `tools inject` 时，程序会：

1. 读取本机公钥文件
2. 使用用户名和密码通过 SSH 连接远程服务器
3. 检查远程 `~/.ssh/authorized_keys` 中是否已存在该公钥
4. 若不存在，则自动追加公钥
5. 在本机 `~/.ssh/config` 中追加如下配置

追加到本地 `~/.ssh/config` 的内容类似：

```sshconfig
Host 192.168.1.10
  HostName 192.168.1.10
  User root
  Port 22
  IdentityFile ~/.ssh/id_rsa
```

如果本地 `~/.ssh/config` 中已存在 `Host <目标IP>`，程序会跳过重复写入。

## 安全说明

当前 `inject` 命令为了方便使用，存在以下特点：

- 通过明文密码建立 SSH 连接
- YAML 配置文件中需要保存明文密码
- 使用了 `ssh.InsecureIgnoreHostKey()`，即跳过主机指纹校验

这意味着它更适合个人内网、临时环境或受信任机器的快速初始化，不建议直接用于高安全要求的生产环境。

如果后续要继续完善，比较值得优先做的方向有：

- 支持从环境变量读取密码
- 支持交互式输入密码而不是写入配置文件
- 校验并记录服务器 host key
- 支持 `ed25519` 默认密钥路径
- 为命令增加测试和更明确的错误提示

## 项目结构

```text
.
├── main.go
├── cmd/
│   ├── root.go
│   ├── ls.go
│   ├── sysinfo.go
│   └── inject.go
├── bin/
│   └── tools
└── README.md
```

## 开发说明

- CLI 框架：`github.com/spf13/cobra`
- 配置读取：`github.com/spf13/viper`
- SSH 连接：`golang.org/x/crypto/ssh`

如需查看帮助：

```bash
go run . --help
go run . ls --help
go run . inject --help
```
