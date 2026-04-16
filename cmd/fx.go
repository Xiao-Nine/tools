/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	fxUpload   bool   // 上传标志
	fxOneTime  bool   // 一次性文件
	fxExpire   string // 过期时间
)

// FxConfig 存储fx_server配置
type FxConfig struct {
	Server string `mapstructure:"server"`
}

// UploadResponse 上传响应结构
type UploadResponse struct {
	Code      string `json:"code"`
	Filename  string `json:"filename"`
	ExpiresAt string `json:"expiresAt"`
	OneTime   bool   `json:"oneTime"`
	Msg       string `json:"msg"`
}

// fxCmd represents the fx command
var fxCmd = &cobra.Command{
	Use:   "fx [-s <path> | <code>]",
	Short: "File exchange with fx_server",
	Long: `Upload or download files from fx_server.

Upload mode (with -s flag):
  fx -s <file_or_directory>
  Uploads file or directory to fx_server and displays the 6-digit pickup code.
  Directories are automatically compressed into a zip file before upload.

Download mode (without -s flag):
  fx <code>
  Downloads file from fx_server using the 6-digit pickup code.
  If the file is a zip archive containing a directory, it will be auto-extracted.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// 读取配置
		config, err := loadFxConfig()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		if fxUpload {
			// 上传模式
			if len(args) == 0 {
				fmt.Println("Please specify the file or directory to upload")
				os.Exit(1)
			}
			path := args[0]
			uploadToServer(config.Server, path)
		} else {
			// 下载模式
			if len(args) == 0 {
				fmt.Println("Please specify the pickup code to download")
				os.Exit(1)
			}
			code := args[0]
			downloadFromServer(config.Server, code)
		}
	},
}

func init() {
	rootCmd.AddCommand(fxCmd)

	fxCmd.Flags().BoolVarP(&fxUpload, "send", "s", false, "upload file or directory to fx_server")
	fxCmd.Flags().BoolVarP(&fxOneTime, "one-time", "o", true, "one-time file (default: true)")
	fxCmd.Flags().StringVarP(&fxExpire, "expire", "e", "24h", "expiration time (default: 24h)")
}

// loadFxConfig 加载配置
func loadFxConfig() (*FxConfig, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %v", err)
	}

	configPath := filepath.Join(usr.HomeDir, ".tools", "config.yaml")

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v\nPlease create the config file with 'fx_server' setting", configPath, err)
	}

	var config FxConfig
	if err := viper.UnmarshalKey("fx_server", &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	if config.Server == "" {
		return nil, fmt.Errorf("fx_server is not configured in %s", configPath)
	}

	return &config, nil
}

// uploadToServer 上传文件或目录到服务器
func uploadToServer(serverURL string, path string) {
	// 检查路径是否存在
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("Error: path '%s' does not exist: %v\n", path, err)
		os.Exit(1)
	}

	var filepathToUpload string
	var originalName string = filepath.Base(path)
	var displayName string = filepath.Base(path)

	// 如果是目录，先压缩成zip
	if info.IsDir() {
		fmt.Printf("Compressing directory '%s'...\n", path)
		tempZip, err := createTempZip(path)
		if err != nil {
			fmt.Printf("Failed to compress directory: %v\n", err)
			os.Exit(1)
		}
		filepathToUpload = tempZip
		originalName = filepath.Base(path) + ".zip"
		defer os.Remove(tempZip)
	} else {
		filepathToUpload = path
	}

	// 上传文件
	code, expiresAt, err := uploadFile(serverURL, filepathToUpload, originalName, fxOneTime, fxExpire)
	if err != nil {
		fmt.Printf("Upload failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Upload successful!\n")
	fmt.Printf("  Pickup code: %s\n", code)
	fmt.Printf("  File: %s\n", displayName)
	fmt.Printf("  One-time: %v\n", fxOneTime)
	fmt.Printf("  Expires at: %s\n", expiresAt)
}

// createTempZip 将目录压缩成临时zip文件
func createTempZip(dirPath string) (string, error) {
	tempFile, err := os.CreateTemp("", "fx-upload-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tempFile.Close()

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 创建zip文件头
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(filepath.Dir(dirPath), path)
		if err != nil {
			return err
		}

		// 使用正斜杠（zip标准）
		header.Name = strings.ReplaceAll(relPath, "\\", "/")

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			return err
		}

		return nil
	})

	if err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

// uploadFile 上传文件到服务器
func uploadFile(serverURL string, filePath string, displayFileName string, oneTime bool, expire string) (string, string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// 创建multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 添加 oneTime 字段
	err = writer.WriteField("oneTime", fmt.Sprintf("%v", oneTime))
	if err != nil {
		return "", "", fmt.Errorf("failed to write oneTime field: %v", err)
	}

	// 添加 expire 字段
	err = writer.WriteField("expire", expire)
	if err != nil {
		return "", "", fmt.Errorf("failed to write expire field: %v", err)
	}

	// 创建file字段，使用显示名称
	part, err := writer.CreateFormFile("file", displayFileName)
	if err != nil {
		return "", "", fmt.Errorf("failed to create form file: %v", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return "", "", fmt.Errorf("failed to copy file content: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return "", "", fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// 发送请求
	uploadURL := fmt.Sprintf("%s/api/files/upload", serverURL)
	req, err := http.NewRequest("POST", uploadURL, &requestBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %v", err)
	}

	return uploadResp.Code, uploadResp.ExpiresAt, nil
}

// downloadFromServer 从服务器下载文件
func downloadFromServer(serverURL string, code string) {
	// 验证code格式（6位数字）
	if len(code) != 6 {
		fmt.Println("Error: pickup code must be 6 digits")
		os.Exit(1)
	}

	if _, err := strconv.Atoi(code); err != nil {
		fmt.Println("Error: pickup code must be numeric")
		os.Exit(1)
	}

	// 下载文件
	filePath, err := downloadFile(serverURL, code)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Download successful!\n")
	fmt.Printf("  Saved to: %s\n", filePath)

	// 检查是否是zip文件，如果是则解压
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		fmt.Printf("\nDetected zip file, extracting...\n")
		extractDir := strings.TrimSuffix(filePath, ".zip")
		if err := extractZip(filePath, extractDir); err != nil {
			fmt.Printf("Warning: failed to extract zip file: %v\n", err)
		} else {
			os.Remove(filePath)
			fmt.Printf("✓ Extracted to: %s\n", extractDir)
		}
	}
}

// downloadFile 下载文件
func downloadFile(serverURL string, code string) (string, error) {
	downloadURL := fmt.Sprintf("%s/api/files/download/%s", serverURL, code)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("pickup code not found or file expired")
	}

	if resp.StatusCode == http.StatusGone {
		return "", fmt.Errorf("file has expired")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// 从Content-Disposition获取文件名
	filename := getFilenameFromResponse(resp)
	if filename == "" {
		// 调试：打印响应头信息
		fmt.Printf("Warning: Could not extract filename from server response\n")
		fmt.Printf("Content-Disposition: %s\n", resp.Header.Get("Content-Disposition"))
		filename = "downloaded_file"
	}

	fmt.Printf("Downloading: %s\n", filename)

	// 如果文件已存在，添加数字后缀
	finalPath := filename
	counter := 1
	for {
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(filename)
		name := strings.TrimSuffix(filename, ext)
		finalPath = fmt.Sprintf("%s_%d%s", name, counter, ext)
		counter++
	}

	// 保存文件
	out, err := os.Create(finalPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %v", err)
	}

	return finalPath, nil
}

// getFilenameFromResponse 从HTTP响应中提取文件名
func getFilenameFromResponse(resp *http.Response) string {
	contentDisp := resp.Header.Get("Content-Disposition")
	if contentDisp == "" {
		return ""
	}

	// Content-Disposition 格式通常是:
	// attachment; filename="file.zip"
	// 或 attachment; filename=file.zip

	// 尝试解析 filename="..." 格式
	if idx := strings.Index(contentDisp, `filename="`); idx != -1 {
		rest := contentDisp[idx+10:] // filename=" 是10个字符
		if endIdx := strings.Index(rest, `"`); endIdx != -1 {
			return rest[:endIdx]
		}
	}

	// 尝试解析 filename=... 格式 (无引号)
	if idx := strings.Index(contentDisp, `filename=`); idx != -1 {
		rest := contentDisp[idx+9:] // filename= 是9个字符
		// 去除可能的空格和分号
		rest = strings.TrimSpace(rest)
		if endIdx := strings.Index(rest, ";"); endIdx != -1 {
			return rest[:endIdx]
		}
		return rest
	}

	// 尝试解析 filename*="..." 格式 (RFC 5987)
	if idx := strings.Index(contentDisp, `filename*=`); idx != -1 {
		rest := contentDisp[idx+10:]
		// 格式: UTF-8''filename
		if parts := strings.SplitN(rest, "''", 2); len(parts) == 2 {
			return parts[1]
		}
	}

	return ""
}

// extractZip 解压zip文件
func extractZip(zipPath string, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %v", err)
	}
	defer reader.Close()

	// 创建目标目录
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	for _, file := range reader.File {
		// 构建目标路径
		path := filepath.Join(destDir, file.Name)

		// 确保路径在目标目录内（防止zip slip攻击）
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.FileInfo().Mode())
			continue
		}

		// 创建父目录
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		// 解压文件
		destFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			destFile.Close()
			return err
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
}