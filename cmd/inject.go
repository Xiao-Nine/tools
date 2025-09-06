/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type ServerConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

var (
	pubKeyFile  string // 公钥文件
	serversFile string // 服务器配置文件
	connInfo    string // 单个服务器的连接字符串：username:password@ip:port
)

// injectCmd represents the inject command
var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject public key into remote servers",
	Long:  `Inject public key into remote servers`,
	Run: func(cmd *cobra.Command, args []string) {
		if pubKeyFile == "" {
			usr, err := user.Current()
			if err != nil {
				fmt.Println("Failed to get current user:", err)
				os.Exit(1)
			}
			pubKeyFile = filepath.Join(usr.HomeDir, ".ssh", "id_rsa.pub")
			if _, err := os.Stat(pubKeyFile); os.IsNotExist(err) {
				fmt.Printf("Public key file %s does not exist\n", pubKeyFile)
				fmt.Println("Please specify a valid public key file using --pubkey flag")
				os.Exit(1)
			}
		}
		pubKey, err := os.ReadFile(pubKeyFile)
		if err != nil {
			fmt.Println("Error reading public key file:", err)
			os.Exit(1)
		}
		if connInfo != "" {
			// 处理单个连接
			server, err := parseConnInfo(connInfo)
			if err != nil {
				fmt.Println("Invalid connection info:", err)
				os.Exit(1)
			}
			if err := injectPublicKey(server, pubKey); err != nil {
				fmt.Printf("Failed to inject key into %s: %v\n", server.Host, err)
			} else {
				fmt.Printf("Successfully injected key into %s\n", server.Host)
			}
		} else if serversFile != "" {
			// 批量处理
			var servers []ServerConfig

			viper.SetConfigFile(serversFile)
			viper.SetConfigType("yaml")
			if err := viper.ReadInConfig(); err != nil {
				fmt.Println("Error reading servers config file:", err)
				os.Exit(1)
			}

			if err := viper.UnmarshalKey("servers", &servers); err != nil {
				fmt.Println("Error unmarshalling servers:", err)
				os.Exit(1)
			}

			for _, server := range servers {
				if err := injectPublicKey(server, pubKey); err != nil {
					fmt.Printf("Failed to inject key into %s: %v\n", server.Host, err)
				} else {
					fmt.Printf("Successfully injected key into %s\n", server.Host)
				}
			}
		} else {
			// 参数错误
			fmt.Println("Please specify either --conn or --servers")
			os.Exit(1)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	injectCmd.Flags().StringVar(&pubKeyFile, "pubkey", "", "public key file (default is ~/.ssh/id_rsa.pub)")
	injectCmd.Flags().StringVar(&serversFile, "servers", "", "servers config file (YAML format, example:\nservers:\n  - host: \"192.168.1.1\"\n    port: \"22\"\n    username: \"user\"\n    password: \"pass\"\n  - host: \"192.168.1.2\"\n    port: \"22\"\n    username: \"root\"\n    password: \"password\")")
	injectCmd.Flags().StringVar(&connInfo, "server", "", "single server connection info (username:password@ip:port)")

	rootCmd.AddCommand(injectCmd)

}
func initConfig() {
	// 当前没啥用，可以拓展全局配置
}

// parseConnInfo 解析类似 username:password@ip:port 的连接字符串
func parseConnInfo(info string) (ServerConfig, error) {
	parts := strings.Split(info, "@")
	if len(parts) != 2 {
		return ServerConfig{}, fmt.Errorf("invalid conn format, should be username:password@ip:port")
	}

	authParts := strings.SplitN(parts[0], ":", 2)
	if len(authParts) != 2 {
		return ServerConfig{}, fmt.Errorf("invalid auth format, should be username:password")
	}

	addrParts := strings.SplitN(parts[1], ":", 2)
	if len(addrParts) < 1 {
		return ServerConfig{}, fmt.Errorf("invalid address format, should be ip:port")
	}
	port := "22"
	if len(addrParts) == 2 {
		port = addrParts[1] // If port is specified, use it
	}
	return ServerConfig{
		Username: authParts[0],
		Password: authParts[1],
		Host:     addrParts[0],
		Port:     port,
	}, nil
}

// injectPublicKey 连接服务器并注入公钥
func injectPublicKey(config ServerConfig, pubKey []byte) error {
	sshConfig := &ssh.ClientConfig{
		User: config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 跳过 host key 校验
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", config.Host, config.Port), sshConfig)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()
	pubKeyStr := strings.TrimSpace(string(pubKey))
	// 确保 .ssh 目录存在并设置权限，然后把公钥写入 authorized_keys
	checkCmd := fmt.Sprintf("grep -qxF \"%s\" ~/.ssh/authorized_keys", pubKeyStr)
	cmd := fmt.Sprintf("mkdir -p ~/.ssh && chmod 700 ~/.ssh && if ! %s; then echo \"%s\" >> ~/.ssh/authorized_keys; fi", checkCmd, pubKeyStr)
	if _, err := session.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("failed to inject public key: %v", err)
	}
	// 将服务器IP添加到本地.ssh/config文件
	if err := addToSshConfig(config); err != nil {
		fmt.Printf("Warning: Failed to add %s to ssh config: %v\n", config.Host, err)
	}

	return nil
}

// addToSshConfig 将服务器信息添加到用户的.ssh/config文件中
func addToSshConfig(config ServerConfig) error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %v", err)
	}

	sshConfigPath := filepath.Join(usr.HomeDir, ".ssh", "config")
	identityFilePath := filepath.Join(usr.HomeDir, ".ssh", "id_rsa")

	// 确保.ssh目录存在
	sshDir := filepath.Join(usr.HomeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %v", err)
	}

	// 检查配置是否已存在
	configContent, err := os.ReadFile(sshConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read ssh config: %v", err)
	}

	// 构建主机配置条目，包含IdentityFile但不包含密码
	hostConfig := fmt.Sprintf("\nHost %s\n  HostName %s\n  User %s\n  Port %s\n  IdentityFile %s\n",
		config.Host, config.Host, config.Username, config.Port, identityFilePath)

	// 检查配置是否已存在
	if strings.Contains(string(configContent), fmt.Sprintf("Host %s", config.Host)) {
		return nil // 配置已存在，无需添加
	}

	// 追加配置到文件
	f, err := os.OpenFile(sshConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open ssh config file: %v", err)
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			fmt.Printf("Failed to close ssh config file: %v\n", err)
		}
	}(f)

	if _, err := f.WriteString(hostConfig); err != nil {
		return fmt.Errorf("failed to write to ssh config: %v", err)
	}

	return nil
}
