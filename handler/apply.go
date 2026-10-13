package handler

import (
	"encoding/json"
	"net/http"
)

type H map[string]any

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// setNoCache 设置禁止缓存的响应头。用于设备列表等动态数据接口，
// 避免浏览器或反向代理缓存响应而返回过期数据。
func setNoCache(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
}
