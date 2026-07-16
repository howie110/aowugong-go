// Package app 负责组装应用运行时。
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const migrationDirectoryName = "migrations/mysql"

// resolveMigrationsDirectory 按运行环境解析应使用的迁移目录。
func resolveMigrationsDirectory(environment, configuredDirectory, executablePath string) (string, error) {
	// 1. 显式配置始终覆盖自动解析结果。
	if configuredDirectory != "" {
		return filepath.Clean(configuredDirectory), nil
	}

	// 2. 优先使用可执行文件同级的生产迁移目录。
	executableDirectory := filepath.Join(filepath.Dir(executablePath), migrationDirectoryName)
	if info, err := os.Stat(executableDirectory); err == nil && info.IsDir() {
		return executableDirectory, nil
	}

	// 3. 生产与未知环境禁止使用编译时源码目录。
	if environment == "production" {
		return "", fmt.Errorf("生产环境缺少迁移目录: %s", executableDirectory)
	}
	if environment != "development" && environment != "test" {
		return "", fmt.Errorf("环境 %q 不允许源码迁移目录回退", environment)
	}

	// 4. 从当前源码文件定位仓库根目录中的迁移目录。
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位源码迁移目录")
	}
	sourceDirectory := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", migrationDirectoryName))

	// 5. 验证源码迁移目录存在且为目录。
	info, err := os.Stat(sourceDirectory)
	if err != nil {
		return "", fmt.Errorf("检查源码迁移目录: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("源码迁移路径不是目录: %s", sourceDirectory)
	}
	return sourceDirectory, nil
}
