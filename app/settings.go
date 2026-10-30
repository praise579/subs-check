package app

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/beck-8/subs-check/config"
	"github.com/beck-8/subs-check/utils"
	"github.com/gin-gonic/gin"
)

// patchConfig 把 values 里给出的顶层键写回配置文件，其余字节原样保留。
// 写之前留一份 <path>.bak；配置文件监听器随后会自动热重载。
func (app *App) patchConfig(values map[string]any) (*config.PatchResult, error) {
	src, err := os.ReadFile(app.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	changed, err := config.PatchChanged(src, values)
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return &config.PatchResult{}, nil
	}

	patched, err := config.PatchYAML(src, values)
	if err != nil {
		return nil, err
	}

	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(app.configPath); statErr == nil {
		mode = fi.Mode().Perm()
	}
	backupPath := app.configPath + ".bak"
	if err := os.WriteFile(backupPath, src, mode); err != nil {
		return nil, fmt.Errorf("写入配置备份失败: %w", err)
	}
	if err := utils.WriteFileAtomic(app.configPath, patched); err != nil {
		return nil, fmt.Errorf("写入配置文件失败: %w", err)
	}

	return &config.PatchResult{
		Changed:      changed,
		NeedsRestart: config.RequiresRestart(changed),
		BackupPath:   backupPath,
	}, nil
}

// updateSettings 只改写请求体里给出的顶层键，返回实际改动、需重启的键与备份位置。
// 前端基础设置表单用这个接口，不再整份 YAML 覆盖写入。
func (app *App) updateSettings(c *gin.Context) {
	var values map[string]any
	if err := c.ShouldBindJSON(&values); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求格式"})
		return
	}
	if len(values) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要修改的配置项"})
		return
	}

	res, err := app.patchConfig(values)
	switch {
	case err == nil:
	case errors.Is(err, config.ErrBadKey), errors.Is(err, config.ErrNotMapping):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"changed":      res.Changed,
		"needsRestart": res.NeedsRestart,
		"backup":       res.BackupPath,
	})
}
