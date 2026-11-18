package server

// 报告资产（Logo）上传与只读访问。文件落在 keyDir/report_assets/assets/，
// 以内容 sha256 命名，避免用户可控文件名；不进可浏览的 dataDir。
// 模板/报告设置里的 logo_path 保存相对路径（assets/<sha>.<ext>）。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	reportAssetMaxBytes = 10 << 20 // Logo 上限 10MB
	reportAssetDirName  = "assets"
)

var reportAssetNameRe = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|gif)$`)

func (s *Server) reportAssetsDir() string {
	return filepath.Join(s.keyDir, "report_assets", reportAssetDirName)
}

// reportAssetName 校验并归一化对外文件名（只接受 <sha256>.<ext>）。
func reportAssetName(raw string) (string, bool) {
	name := path.Base(strings.TrimSpace(raw))
	return name, reportAssetNameRe.MatchString(name)
}

// resolveReportAssetPath 把 logo_path 解析成磁盘绝对路径并确认仍在
// keyDir/report_assets 之内。兼容历史写法（report_assets/logo.png、绝对路径）。
func (s *Server) resolveReportAssetPath(logoPath string) (string, bool) {
	raw := strings.TrimSpace(logoPath)
	if raw == "" {
		return "", false
	}
	// 支持前端直接存 /api/report-assets/<file> 的形式。
	if i := strings.Index(raw, "/api/report-assets/"); i >= 0 {
		raw = raw[i+len("/api/report-assets/"):]
	}
	base := filepath.Join(s.keyDir, "report_assets")
	var candidates []string
	switch {
	case strings.HasPrefix(raw, "report_assets/"):
		candidates = append(candidates, filepath.Join(s.keyDir, raw))
	case strings.HasPrefix(raw, "assets/"):
		candidates = append(candidates, filepath.Join(base, raw))
	default:
		if name, ok := reportAssetName(raw); ok {
			candidates = append(candidates, filepath.Join(s.reportAssetsDir(), name))
		}
		candidates = append(candidates, filepath.Join(base, raw))
	}
	for _, c := range candidates {
		abs := filepath.Clean(c)
		if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
			continue
		}
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return abs, true
		}
	}
	return "", false
}

// readReportLogo 读取 logo_path 指向的图片字节；缺失/不可读返回 nil。
func (s *Server) readReportLogo(logoPath string) []byte {
	abs, ok := s.resolveReportAssetPath(logoPath)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// uploadReportLogo POST /api/report-assets/logo（multipart file）。
func (s *Server) uploadReportLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, reportAssetMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(reportAssetMaxBytes); err != nil {
		writeErr(w, 400, "上传解析失败")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少 file 字段")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, reportAssetMaxBytes+1))
	if err != nil {
		writeErr(w, 400, "读取文件失败")
		return
	}
	if len(data) == 0 {
		writeErr(w, 400, "文件为空")
		return
	}
	if len(data) > reportAssetMaxBytes {
		writeErr(w, 413, "Logo 文件超过 10MB 上限")
		return
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		writeErr(w, 400, "仅支持 png/jpeg/gif 图片")
		return
	}
	ext := "png"
	switch format {
	case "jpeg":
		ext = "jpg"
	case "gif":
		ext = "gif"
	case "png":
		ext = "png"
	default:
		writeErr(w, 400, "仅支持 png/jpeg/gif 图片")
		return
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + "." + ext
	dir := s.reportAssetsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, 500, "创建目录失败: "+err.Error())
		return
	}
	out := filepath.Join(dir, name)
	if _, err := os.Stat(out); err != nil {
		if err := os.WriteFile(out, data, 0o600); err != nil {
			writeErr(w, 500, "保存失败: "+err.Error())
			return
		}
	}
	original := ""
	if hdr != nil {
		original = filepath.Base(hdr.Filename)
	}
	writeJSON(w, 200, map[string]any{
		"path":   "assets/" + name,
		"name":   name,
		"file":   name,
		"url":    "/api/report-assets/" + name,
		"size":   len(data),
		"sha256": hex.EncodeToString(sum[:]),
		"source": original,
	})
}

// serveReportAsset GET /api/report-assets/{file}：只读回放已上传的 Logo。
func (s *Server) serveReportAsset(w http.ResponseWriter, r *http.Request) {
	name, ok := reportAssetName(r.PathValue("file"))
	if !ok {
		writeErr(w, 400, "非法文件名")
		return
	}
	abs := filepath.Join(s.reportAssetsDir(), name)
	if _, err := os.Stat(abs); err != nil {
		writeErr(w, 404, "文件不存在")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, abs)
}

// normalizeReportAssetPath 供模板保存时统一 logo_path 写法。
func normalizeReportAssetPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "/api/report-assets/"); i >= 0 {
		return "assets/" + path.Base(raw[i+len("/api/report-assets/"):])
	}
	if name, ok := reportAssetName(raw); ok {
		return "assets/" + name
	}
	// 兼容历史自由文本路径：保留原值，由 resolveReportAssetPath 兜底解析。
	return raw
}
